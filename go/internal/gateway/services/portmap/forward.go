package portmap

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	auditstore "github.com/ops-bastion/ops/go/internal/gateway/infra/audit"
	"github.com/ops-bastion/ops/go/internal/protocol/datagram"
	"github.com/ops-bastion/ops/go/internal/sessionws"
)

func (s *Service) serveTCP(ctx context.Context, e *entry) {
	for {
		conn, err := e.ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("portmap accept error mapping=%s: %v", e.runtime.MappingID, err)
				return
			}
		}
		go s.handleTCPConn(ctx, e, conn)
	}
}

func (s *Service) handleTCPConn(ctx context.Context, e *entry, client net.Conn) {
	defer client.Close()
	clientAddr := client.RemoteAddr().String()
	var opened map[string]any
	if err := s.deps.PostJSON("/api/internal/port-mappings/connections/open-record", map[string]any{
		"mappingId": e.runtime.MappingID, "clientAddr": clientAddr,
	}, &opened); err != nil {
		log.Printf("portmap open-record failed: %v", err)
		return
	}
	sessionID, ticket, assetID := str(opened["sessionId"]), str(opened["ticket"]), str(opened["assetId"])
	if assetID == "" {
		assetID = e.runtime.AssetID
	}
	detail := e.auditDetail(clientAddr)
	atomic.AddInt64(&e.runtime.ActiveConns, 1)
	defer atomic.AddInt64(&e.runtime.ActiveConns, -1)

	type waitResult struct {
		ws  *websocket.Conn
		err error
	}
	s.deps.Arm(sessionID)
	waitDone := make(chan waitResult, 1)
	go func() {
		ws, err := s.deps.Wait(sessionID, 20*time.Second)
		waitDone <- waitResult{ws, err}
	}()
	if err := s.deps.OpenSession(assetID, sessionID, ticket, "tcp",
		e.runtime.TargetHost, e.runtime.TargetPort); err != nil {
		log.Printf("portmap open_session failed: %v", err)
		s.deps.AuditEnd(auditstore.TypePortmapTCP, sessionID, assetID, false,
			mergeDetail(detail, map[string]any{"error": err.Error()}))
		return
	}
	wr := <-waitDone
	if wr.err != nil {
		log.Printf("portmap agent session wait failed: %v", wr.err)
		s.deps.AuditEnd(auditstore.TypePortmapTCP, sessionID, assetID, false,
			mergeDetail(detail, map[string]any{"error": wr.err.Error()}))
		return
	}
	agentWS := wr.ws
	defer agentWS.Close()
	s.deps.AuditStart(auditstore.TypePortmapTCP, sessionID, assetID, detail)

	tunnel := sessionws.NewBinary(agentWS)
	var bytesIn, bytesOut int64
	errCh := make(chan error, 2)
	go func() {
		n, err := copyPortMap(tunnel, client)
		atomic.AddInt64(&e.runtime.BytesIn, n)
		atomic.AddInt64(&bytesIn, n)
		errCh <- err
	}()
	go func() {
		n, err := copyPortMap(client, tunnel)
		atomic.AddInt64(&e.runtime.BytesOut, n)
		atomic.AddInt64(&bytesOut, n)
		errCh <- err
	}()

	var firstErr error
	select {
	case <-ctx.Done():
		firstErr = ctx.Err()
	case firstErr = <-errCh:
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
		case <-errCh:
		case <-timer.C:
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	_ = client.Close()
	_ = agentWS.Close()
	inTotal, outTotal := atomic.LoadInt64(&bytesIn), atomic.LoadInt64(&bytesOut)
	log.Printf("portmap tcp connection closed mapping=%s client=%s bytesIn=%d bytesOut=%d firstErr=%v",
		e.runtime.MappingID, clientAddr, inTotal, outTotal, firstErr)
	s.deps.AuditEnd(auditstore.TypePortmapTCP, sessionID, assetID, true,
		mergeDetail(detail, map[string]any{"bytesIn": inTotal, "bytesOut": outTotal}))
}

func (s *Service) serveUDP(ctx context.Context, e *entry) {
	var opened map[string]any
	if err := s.deps.PostJSON("/api/internal/port-mappings/connections/open-record", map[string]any{
		"mappingId": e.runtime.MappingID, "clientAddr": "udp-mapping",
	}, &opened); err != nil {
		log.Printf("portmap udp open-record failed: %v", err)
		s.close(e.runtime.MappingID)
		return
	}
	sessionID, ticket, assetID := str(opened["sessionId"]), str(opened["ticket"]), str(opened["assetId"])
	if assetID == "" {
		assetID = e.runtime.AssetID
	}
	detail := e.auditDetail("udp-mapping")
	s.deps.Arm(sessionID)
	waitDone := make(chan struct {
		ws  *websocket.Conn
		err error
	}, 1)
	go func() {
		ws, err := s.deps.Wait(sessionID, 20*time.Second)
		waitDone <- struct {
			ws  *websocket.Conn
			err error
		}{ws, err}
	}()
	if err := s.deps.OpenSession(assetID, sessionID, ticket, "udp",
		e.runtime.TargetHost, e.runtime.TargetPort); err != nil {
		log.Printf("portmap udp open_session failed: %v", err)
		s.deps.AuditEnd(auditstore.TypePortmapUDP, sessionID, assetID, false,
			mergeDetail(detail, map[string]any{"error": err.Error()}))
		s.close(e.runtime.MappingID)
		return
	}
	wr := <-waitDone
	if wr.err != nil {
		log.Printf("portmap udp agent wait failed: %v", wr.err)
		s.deps.AuditEnd(auditstore.TypePortmapUDP, sessionID, assetID, false,
			mergeDetail(detail, map[string]any{"error": wr.err.Error()}))
		s.close(e.runtime.MappingID)
		return
	}
	agentWS := wr.ws
	defer agentWS.Close()
	atomic.StoreInt64(&e.runtime.ActiveConns, 1)
	defer atomic.StoreInt64(&e.runtime.ActiveConns, 0)
	s.deps.AuditStart(auditstore.TypePortmapUDP, sessionID, assetID, detail)

	var bytesIn, bytesOut int64
	errCh := make(chan error, 2)
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, addr, err := e.pc.ReadFrom(buf)
			if err != nil {
				errCh <- err
				return
			}
			host, portStr, _ := net.SplitHostPort(addr.String())
			port := 0
			fmt.Sscanf(portStr, "%d", &port)
			if err := agentWS.WriteMessage(websocket.BinaryMessage, datagram.Encode(host, port, buf[:n])); err != nil {
				errCh <- err
				return
			}
			atomic.AddInt64(&e.runtime.BytesIn, int64(n))
			atomic.AddInt64(&bytesIn, int64(n))
		}
	}()
	go func() {
		for {
			_, data, err := agentWS.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			host, port, payload, err := datagram.Decode(data)
			if err != nil {
				continue
			}
			raddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
			if err != nil {
				continue
			}
			n, err := e.pc.WriteTo(payload, raddr)
			if err != nil {
				errCh <- err
				return
			}
			atomic.AddInt64(&e.runtime.BytesOut, int64(n))
			atomic.AddInt64(&bytesOut, int64(n))
		}
	}()
	select {
	case <-ctx.Done():
	case <-errCh:
	}
	_ = agentWS.Close()
	s.deps.AuditEnd(auditstore.TypePortmapUDP, sessionID, assetID, true,
		mergeDetail(detail, map[string]any{
			"bytesIn": atomic.LoadInt64(&bytesIn), "bytesOut": atomic.LoadInt64(&bytesOut),
		}))
}

func copyPortMap(dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			wn, werr := dst.Write(buf[:n])
			total += int64(wn)
			if werr != nil {
				return total, werr
			}
		}
		if err != nil {
			if err == io.EOF {
				return total, nil
			}
			return total, err
		}
	}
}

func mergeDetail(base, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
