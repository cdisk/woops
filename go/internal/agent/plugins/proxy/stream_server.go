package proxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
)

type singleConnListener struct {
	conn     net.Conn
	once     sync.Once
	done     chan struct{}
	connDone chan struct{}
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	var conn net.Conn
	l.once.Do(func() { conn = l.conn })
	if conn != nil {
		return conn, nil
	}
	select {
	case <-l.done:
	case <-l.connDone:
	}
	return nil, net.ErrClosed
}

func (l *singleConnListener) Close() error {
	select {
	case <-l.done:
	default:
		close(l.done)
	}
	return nil
}

func (l *singleConnListener) Addr() net.Addr { return l.conn.LocalAddr() }

type notifyingConn struct {
	net.Conn
	once sync.Once
	done chan struct{}
}

func (c *notifyingConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { close(c.done) })
	return err
}

func serveSingleHTTPConn(ctx context.Context, conn net.Conn, handler httpHandler) {
	connDone := make(chan struct{})
	tracked := &notifyingConn{Conn: conn, done: connDone}
	ln := &singleConnListener{conn: tracked, done: make(chan struct{}), connDone: connDone}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			// Stream errors are intentionally isolated to this carrier stream.
		}
	}
	_ = conn.Close()
	_ = ln.Close()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	_ = srv.Shutdown(shutdownCtx)
	cancel()
	select {
	case <-errCh:
	default:
	}
}
