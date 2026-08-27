package proxy

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
)

const (
	bridgeVersion          = byte(1)
	bridgeNonceSize        = 32
	bridgeProofSize        = sha256.Size
	bridgeHandshakeTimeout = 10 * time.Second
)

var bridgeMagic = [8]byte{'W', 'O', 'O', 'P', 'S', 'P', 'X', 'Y'}

type carrierPool struct {
	mu       sync.RWMutex
	sessions map[*yamux.Session]struct{}
	next     uint64
}

func newCarrierPool() *carrierPool {
	return &carrierPool{sessions: make(map[*yamux.Session]struct{})}
}

func (p *carrierPool) add(session *yamux.Session) {
	p.mu.Lock()
	p.sessions[session] = struct{}{}
	p.mu.Unlock()
}

func (p *carrierPool) remove(session *yamux.Session) {
	p.mu.Lock()
	delete(p.sessions, session)
	p.mu.Unlock()
}

func (p *carrierPool) available() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.sessions) != 0
}

func (p *carrierPool) open() (net.Conn, error) {
	p.mu.RLock()
	sessions := make([]*yamux.Session, 0, len(p.sessions))
	for session := range p.sessions {
		sessions = append(sessions, session)
	}
	p.mu.RUnlock()
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no parent carrier")
	}
	p.mu.Lock()
	start := int(p.next % uint64(len(sessions)))
	p.next++
	p.mu.Unlock()
	for i := range sessions {
		session := sessions[(start+i)%len(sessions)]
		stream, err := session.Open()
		if err == nil {
			return stream, nil
		}
		p.remove(session)
	}
	return nil, fmt.Errorf("parent carrier unavailable")
}

func (p *carrierPool) close() {
	p.mu.Lock()
	sessions := make([]*yamux.Session, 0, len(p.sessions))
	for session := range p.sessions {
		sessions = append(sessions, session)
	}
	p.sessions = make(map[*yamux.Session]struct{})
	p.mu.Unlock()
	for _, session := range sessions {
		_ = session.Close()
	}
}

func yamuxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = 30 * time.Second
	cfg.ConnectionWriteTimeout = 10 * time.Second
	cfg.LogOutput = io.Discard
	return cfg
}

func setCarrierTCPOptions(conn net.Conn) {
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(30 * time.Second)
	}
}

func bridgeProof(key []byte, role byte, listenerNonce, dialerNonce []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte{role})
	_, _ = mac.Write(listenerNonce)
	_, _ = mac.Write(dialerNonce)
	return mac.Sum(nil)
}

func writeBridgeFrame(w io.Writer, parts ...[]byte) error {
	for _, part := range parts {
		if _, err := w.Write(part); err != nil {
			return err
		}
	}
	return nil
}

func listenerHandshake(conn net.Conn, key []byte) error {
	_ = conn.SetDeadline(time.Now().Add(bridgeHandshakeTimeout))
	defer conn.SetDeadline(time.Time{})
	listenerNonce := make([]byte, bridgeNonceSize)
	if _, err := rand.Read(listenerNonce); err != nil {
		return err
	}
	if err := writeBridgeFrame(conn, bridgeMagic[:], []byte{bridgeVersion}, listenerNonce); err != nil {
		return err
	}
	reply := make([]byte, len(bridgeMagic)+1+bridgeNonceSize+bridgeProofSize)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return err
	}
	if !hmac.Equal(reply[:len(bridgeMagic)], bridgeMagic[:]) || reply[len(bridgeMagic)] != bridgeVersion {
		return fmt.Errorf("protocol mismatch")
	}
	dialerNonce := reply[len(bridgeMagic)+1 : len(bridgeMagic)+1+bridgeNonceSize]
	gotProof := reply[len(bridgeMagic)+1+bridgeNonceSize:]
	wantProof := bridgeProof(key, 'D', listenerNonce, dialerNonce)
	if !hmac.Equal(gotProof, wantProof) {
		return fmt.Errorf("authentication failed")
	}
	return writeBridgeFrame(conn, bridgeProof(key, 'L', listenerNonce, dialerNonce))
}

func dialerHandshake(conn net.Conn, key []byte) error {
	_ = conn.SetDeadline(time.Now().Add(bridgeHandshakeTimeout))
	defer conn.SetDeadline(time.Time{})
	challenge := make([]byte, len(bridgeMagic)+1+bridgeNonceSize)
	if _, err := io.ReadFull(conn, challenge); err != nil {
		return err
	}
	if !hmac.Equal(challenge[:len(bridgeMagic)], bridgeMagic[:]) || challenge[len(bridgeMagic)] != bridgeVersion {
		return fmt.Errorf("protocol mismatch")
	}
	listenerNonce := challenge[len(bridgeMagic)+1:]
	dialerNonce := make([]byte, bridgeNonceSize)
	if _, err := rand.Read(dialerNonce); err != nil {
		return err
	}
	proof := bridgeProof(key, 'D', listenerNonce, dialerNonce)
	if err := writeBridgeFrame(conn, bridgeMagic[:], []byte{bridgeVersion}, dialerNonce, proof); err != nil {
		return err
	}
	gotProof := make([]byte, bridgeProofSize)
	if _, err := io.ReadFull(conn, gotProof); err != nil {
		return err
	}
	if !hmac.Equal(gotProof, bridgeProof(key, 'L', listenerNonce, dialerNonce)) {
		return fmt.Errorf("authentication failed")
	}
	return nil
}

func serveBridgeListener(ctx context.Context, cfg BridgeConfig, parents *carrierPool, plog *log.Logger) error {
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("bridge listen %s: %w", cfg.Listen, err)
	}
	defer ln.Close()
	plog.Printf("proxy bridge listening listen=%s allowGlobal=%v", cfg.Listen, cfg.AllowGlobal)
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	var wg sync.WaitGroup
	defer func() {
		parents.close()
		wg.Wait()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		wg.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			defer conn.Close()
			setCarrierTCPOptions(conn)
			if err := listenerHandshake(conn, cfg.keyBytes); err != nil {
				plog.Printf("proxy bridge peer rejected")
				return
			}
			session, err := yamux.Server(conn, yamuxConfig())
			if err != nil {
				return
			}
			parents.add(session)
			plog.Printf("proxy bridge parent connected")
			select {
			case <-ctx.Done():
				_ = session.Close()
			case <-session.CloseChan():
			}
			parents.remove(session)
			plog.Printf("proxy bridge parent disconnected")
		}(conn)
	}
}

func runBridgeTarget(ctx context.Context, target BridgeTarget, streamHandler httpHandler, plog *log.Logger) {
	delay := time.Second
	for ctx.Err() == nil {
		conn, err := (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", target.Address)
		if err == nil {
			setCarrierTCPOptions(conn)
			err = dialerHandshake(conn, target.keyBytes)
		}
		if err == nil {
			session, sessionErr := yamux.Client(conn, yamuxConfig())
			if sessionErr == nil {
				plog.Printf("proxy bridge target connected address=%s", target.Address)
				delay = time.Second
				serveTargetSession(ctx, session, streamHandler)
				plog.Printf("proxy bridge target disconnected address=%s", target.Address)
			}
		}
		if conn != nil {
			_ = conn.Close()
		}
		if ctx.Err() != nil {
			return
		}
		jitterMax := int64(delay / 2)
		var jitter time.Duration
		if jitterMax > 0 {
			if value, err := rand.Int(rand.Reader, big.NewInt(jitterMax)); err == nil {
				jitter = time.Duration(value.Int64())
			}
		}
		timer := time.NewTimer(delay + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay *= 2
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
	}
}

type httpHandler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

func serveTargetSession(ctx context.Context, session *yamux.Session, streamHandler httpHandler) {
	defer session.Close()
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
		case <-session.CloseChan():
		}
	}()
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		stream, err := session.Accept()
		if err != nil {
			return
		}
		wg.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			serveSingleHTTPConn(ctx, conn, streamHandler)
		}(stream)
	}
}
