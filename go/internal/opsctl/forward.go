package opsctl

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/datagram"
	"github.com/ops-bastion/ops/go/internal/sessioncore"
	"github.com/ops-bastion/ops/go/internal/sessionws"
)

func RunForward(ctx context.Context, client *Client, opts TunnelOptions, logger *log.Logger) error {
	if opts.Protocol == "tcp" {
		return runForwardTCP(ctx, client, opts, logger)
	}
	return runForwardUDP(ctx, client, opts, logger)
}

func runForwardTCP(ctx context.Context, client *Client, opts TunnelOptions, logger *log.Logger) error {
	ln, err := net.Listen("tcp", net.JoinHostPort(opts.ListenHost, strconv.Itoa(opts.ListenPort)))
	if err != nil {
		return err
	}
	defer ln.Close()
	if logger != nil {
		logger.Printf("forward tcp listening on %s", ln.Addr())
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	fatal := make(chan error, 1)
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case permanent := <-fatal:
				return permanent
			default:
			}
			if ctx.Err() != nil {
				return nil
			}
			if temporary, ok := err.(net.Error); ok && temporary.Temporary() {
				continue
			}
			return err
		}
		go func(local net.Conn) {
			err := handleForwardTCP(ctx, client, opts, local)
			if err != nil && logger != nil {
				logger.Printf("forward tcp client %s failed: %v", local.RemoteAddr(), err)
			}
			if IsPermanent(err) {
				select {
				case fatal <- err:
					_ = ln.Close()
				default:
				}
			}
		}(conn)
	}
}

func handleForwardTCP(ctx context.Context, client *Client, opts TunnelOptions, local net.Conn) error {
	defer local.Close()
	ticket, err := client.CreateTicketContext(ctx, "port-forward", opts.meta(local.RemoteAddr().String()))
	if err != nil {
		return err
	}
	ws, err := client.dialWS(ctx, ticket.BrowserWS)
	if err != nil {
		return err
	}
	keepDone := make(chan struct{})
	defer close(keepDone)
	sessioncore.StartKeepAlive(ws, keepDone)
	tunnel := sessionws.NewBinary(ws)
	return sessionws.Pipe(ctx, tunnel, local)
}

type packetSink struct {
	mu sync.RWMutex
	ch chan []byte
}

func (s *packetSink) set(ch chan []byte) {
	s.mu.Lock()
	s.ch = ch
	s.mu.Unlock()
}

func (s *packetSink) offer(frame []byte) {
	s.mu.RLock()
	ch := s.ch
	if ch != nil {
		select {
		case ch <- frame:
		default:
		}
	}
	s.mu.RUnlock()
}

func runForwardUDP(ctx context.Context, client *Client, opts TunnelOptions, logger *log.Logger) error {
	pc, err := net.ListenPacket("udp", net.JoinHostPort(opts.ListenHost, strconv.Itoa(opts.ListenPort)))
	if err != nil {
		return err
	}
	defer pc.Close()
	if logger != nil {
		logger.Printf("forward udp listening on %s", pc.LocalAddr())
	}
	go func() {
		<-ctx.Done()
		_ = pc.Close()
	}()

	var sink packetSink
	go readLocalPackets(ctx, pc, &sink)
	for attempt := 0; ; attempt++ {
		ticket, err := client.CreateTicketContext(ctx, "port-forward", opts.meta(""))
		if err == nil {
			var ws *websocket.Conn
			ws, err = client.dialWS(ctx, ticket.BrowserWS)
			if err == nil {
				if logger != nil {
					logger.Printf("forward udp data channel connected")
				}
				attempt = 0
				err = serveForwardUDP(ctx, pc, ws, &sink)
			}
		}
		sink.set(nil)
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

func readLocalPackets(ctx context.Context, pc net.PacketConn, sink *packetSink) {
	buf := make([]byte, 65535)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		host, portText, err := net.SplitHostPort(addr.String())
		if err != nil {
			continue
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			continue
		}
		frame := datagram.Encode(host, port, append([]byte(nil), buf[:n]...))
		sink.offer(frame)
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func serveForwardUDP(ctx context.Context, pc net.PacketConn, ws *websocket.Conn, sink *packetSink) error {
	defer ws.Close()
	keepDone := make(chan struct{})
	defer close(keepDone)
	sessioncore.StartKeepAlive(ws, keepDone)
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	send := make(chan []byte, 256)
	sink.set(send)
	defer sink.set(nil)
	errCh := make(chan error, 2)
	go func() {
		for {
			select {
			case <-connCtx.Done():
				errCh <- connCtx.Err()
				return
			case frame := <-send:
				if err := ws.WriteMessage(websocket.BinaryMessage, frame); err != nil {
					errCh <- err
					return
				}
			}
		}
	}()
	go func() {
		for {
			messageType, frame, err := ws.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			if messageType != websocket.BinaryMessage {
				continue
			}
			host, port, payload, err := datagram.Decode(frame)
			if err != nil {
				continue
			}
			addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(port)))
			if err != nil {
				continue
			}
			if _, err := pc.WriteTo(payload, addr); err != nil {
				errCh <- fmt.Errorf("local udp write: %w", err)
				return
			}
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		cancel()
		return err
	}
}
