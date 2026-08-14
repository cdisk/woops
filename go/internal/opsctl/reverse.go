package opsctl

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/datagram"
	"github.com/ops-bastion/ops/go/internal/sessioncore"
	"github.com/ops-bastion/ops/go/internal/sessionws"
)

type reverseControlMessage struct {
	Type       string `json:"type"`
	OK         bool   `json:"ok,omitempty"`
	Error      string `json:"error,omitempty"`
	ListenHost string `json:"listenHost,omitempty"`
	ListenPort int    `json:"listenPort,omitempty"`
	RequestID  string `json:"requestId,omitempty"`
	ClientAddr string `json:"clientAddr,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

type reverseOpenResponse struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
	SessionID string `json:"sessionId,omitempty"`
	Ticket    string `json:"ticket,omitempty"`
	Error     string `json:"error,omitempty"`
}

func RunReverse(ctx context.Context, client *Client, opts TunnelOptions, logger *log.Logger) error {
	for attempt := 0; ; attempt++ {
		ticket, err := client.CreateTicketContext(ctx, "port-reverse", opts.meta(""))
		if err == nil {
			var ws *websocket.Conn
			ws, err = client.dialWS(ctx, ticket.BrowserWS)
			if err == nil {
				if logger != nil {
					logger.Printf("reverse control channel connected")
				}
				attempt = 0
				err = serveReverseControl(ctx, client, opts, ws, logger)
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		if IsPermanent(err) {
			return err
		}
		if !waitRetry(ctx, attempt, logger, err) {
			return nil
		}
	}
}

func serveReverseControl(ctx context.Context, client *Client, opts TunnelOptions, ws *websocket.Conn, logger *log.Logger) error {
	defer ws.Close()
	keepDone := make(chan struct{})
	defer close(keepDone)
	sessioncore.StartKeepAlive(ws, keepDone)
	var writeMu sync.Mutex
	fatal := make(chan error, 1)
	closed := make(chan struct{})
	defer close(closed)
	go func() {
		select {
		case <-ctx.Done():
			_ = ws.Close()
		case <-closed:
		}
	}()

	for {
		messageType, raw, err := ws.ReadMessage()
		if err != nil {
			select {
			case permanent := <-fatal:
				return permanent
			default:
				return err
			}
		}
		if messageType != websocket.TextMessage {
			continue
		}
		var msg reverseControlMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "status":
			if !msg.OK {
				if msg.Error == "" {
					msg.Error = "reverse listener rejected"
				}
				return fmt.Errorf("%s", msg.Error)
			}
			if logger != nil {
				logger.Printf("reverse %s listening on %s", opts.Protocol,
					net.JoinHostPort(msg.ListenHost, strconv.Itoa(msg.ListenPort)))
			}
		case "open_request":
			go handleReverseOpen(ctx, client, opts, ws, &writeMu, msg, fatal, logger)
		}
	}
}

func handleReverseOpen(
	ctx context.Context,
	client *Client,
	opts TunnelOptions,
	controlWS *websocket.Conn,
	writeMu *sync.Mutex,
	request reverseControlMessage,
	fatal chan<- error,
	logger *log.Logger,
) {
	meta := opts.meta(request.ClientAddr)
	meta["requestId"] = request.RequestID
	ticket, err := client.CreateTicketContext(ctx, "port-reverse-connection", meta)
	response := reverseOpenResponse{Type: "open_response", RequestID: request.RequestID}
	if err != nil {
		response.Error = err.Error()
	} else {
		response.SessionID = ticket.SessionID
		response.Ticket = ticket.Ticket
	}
	writeMu.Lock()
	_ = controlWS.SetWriteDeadline(time.Now().Add(10 * time.Second))
	writeErr := controlWS.WriteJSON(response)
	writeMu.Unlock()
	if writeErr != nil {
		return
	}
	if err != nil {
		if IsPermanent(err) {
			select {
			case fatal <- err:
				_ = controlWS.Close()
			default:
			}
		}
		if logger != nil {
			logger.Printf("reverse open request %s failed: %v", request.RequestID, err)
		}
		return
	}

	dataWS, err := client.dialWS(ctx, ticket.BrowserWS)
	if err == nil {
		protocol := strings.ToLower(strings.TrimSpace(request.Protocol))
		if protocol == "" {
			protocol = opts.Protocol
		}
		if protocol == "udp" {
			err = serveReverseUDP(ctx, dataWS, opts)
		} else {
			err = serveReverseTCP(ctx, dataWS, opts)
		}
	}
	if err != nil && ctx.Err() == nil && logger != nil {
		logger.Printf("reverse data request %s failed: %v", request.RequestID, err)
	}
}

func serveReverseTCP(ctx context.Context, ws *websocket.Conn, opts TunnelOptions) error {
	defer ws.Close()
	keepDone := make(chan struct{})
	defer close(keepDone)
	sessioncore.StartKeepAlive(ws, keepDone)
	target, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(
		ctx, "tcp", net.JoinHostPort(opts.TargetHost, strconv.Itoa(opts.TargetPort)))
	if err != nil {
		return err
	}
	return sessionws.Pipe(ctx, sessionws.NewBinary(ws), target)
}

type udpTarget struct {
	conn     *net.UDPConn
	cancel   context.CancelFunc
	lastSeen time.Time
}

func serveReverseUDP(ctx context.Context, ws *websocket.Conn, opts TunnelOptions) error {
	defer ws.Close()
	keepDone := make(chan struct{})
	defer close(keepDone)
	sessioncore.StartKeepAlive(ws, keepDone)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-runCtx.Done()
		_ = ws.Close()
	}()

	targetAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(opts.TargetHost, strconv.Itoa(opts.TargetPort)))
	if err != nil {
		return err
	}
	var writeMu sync.Mutex
	var targetsMu sync.Mutex
	targets := make(map[string]udpTarget)
	defer func() {
		targetsMu.Lock()
		defer targetsMu.Unlock()
		for _, target := range targets {
			target.cancel()
			_ = target.conn.Close()
		}
	}()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case now := <-ticker.C:
				cutoff := now.Add(-2 * time.Minute)
				targetsMu.Lock()
				for clientAddr, target := range targets {
					if target.lastSeen.Before(cutoff) {
						target.cancel()
						_ = target.conn.Close()
						delete(targets, clientAddr)
					}
				}
				targetsMu.Unlock()
			}
		}
	}()

	for {
		messageType, frame, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		host, port, payload, err := datagram.Decode(frame)
		if err != nil {
			continue
		}
		clientAddr := net.JoinHostPort(host, strconv.Itoa(port))
		targetsMu.Lock()
		target, ok := targets[clientAddr]
		if !ok {
			conn, err := net.DialUDP("udp", nil, targetAddr)
			if err != nil {
				targetsMu.Unlock()
				continue
			}
			targetCtx, targetCancel := context.WithCancel(runCtx)
			target = udpTarget{conn: conn, cancel: targetCancel, lastSeen: time.Now()}
			targets[clientAddr] = target
			go relayReverseUDPReplies(targetCtx, ws, &writeMu, conn, host, port)
		} else {
			target.lastSeen = time.Now()
			targets[clientAddr] = target
		}
		targetsMu.Unlock()
		if _, err := target.conn.Write(payload); err != nil {
			target.cancel()
			_ = target.conn.Close()
			targetsMu.Lock()
			if current, exists := targets[clientAddr]; exists && current.conn == target.conn {
				delete(targets, clientAddr)
			}
			targetsMu.Unlock()
		}
	}
}

func relayReverseUDPReplies(
	ctx context.Context,
	ws *websocket.Conn,
	writeMu *sync.Mutex,
	conn *net.UDPConn,
	clientHost string,
	clientPort int,
) {
	buf := make([]byte, 65535)
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		frame := datagram.Encode(clientHost, clientPort, buf[:n])
		writeMu.Lock()
		err = ws.WriteMessage(websocket.BinaryMessage, frame)
		writeMu.Unlock()
		if err != nil {
			return
		}
	}
}
