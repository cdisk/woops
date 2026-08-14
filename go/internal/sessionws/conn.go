package sessionws

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Conn adapts a single-session WebSocket into a net.Conn-like byte pipe.
// Each WebSocket binary message is one TCP chunk. No stream multiplexing.
type Conn struct {
	ws         *websocket.Conn
	reader     io.Reader
	mu         sync.Mutex
	closed     bool
	binaryOnly bool
}

func New(ws *websocket.Conn) *Conn {
	return &Conn{ws: ws}
}

func NewBinary(ws *websocket.Conn) *Conn {
	return &Conn{ws: ws, binaryOnly: true}
}

// NewWithFirst returns a Conn that will first yield an already-read websocket message (if any).
func NewWithFirst(ws *websocket.Conn, msgType int, first []byte, firstErr error) *Conn {
	c := &Conn{ws: ws}
	if firstErr != nil {
		return c
	}
	if len(first) == 0 {
		return c
	}
	if msgType == websocket.BinaryMessage || msgType == websocket.TextMessage {
		c.reader = &prefixReader{b: first}
	}
	return c
}

type prefixReader struct {
	b []byte
	i int
}

func (p *prefixReader) Read(buf []byte) (int, error) {
	if p.i >= len(p.b) {
		return 0, io.EOF
	}
	n := copy(buf, p.b[p.i:])
	p.i += n
	if p.i >= len(p.b) {
		return n, io.EOF
	}
	return n, nil
}

func (c *Conn) Read(p []byte) (int, error) {
	for {
		if c.reader != nil {
			n, err := c.reader.Read(p)
			if n > 0 {
				return n, nil
			}
			if err == io.EOF {
				c.reader = nil
				continue
			}
			if err != nil {
				return 0, err
			}
		}
		msgType, r, err := c.ws.NextReader()
		if err != nil {
			return 0, err
		}
		if msgType != websocket.BinaryMessage && msgType != websocket.TextMessage {
			continue
		}
		if c.binaryOnly && msgType != websocket.BinaryMessage {
			return 0, fmt.Errorf("sessionws: expected binary frame, got type %d", msgType)
		}
		c.reader = r
	}
}

func (c *Conn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, net.ErrClosed
	}
	if err := c.ws.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return c.ws.Close()
}

func (c *Conn) LocalAddr() net.Addr                { return dummyAddr("session-local") }
func (c *Conn) RemoteAddr() net.Addr               { return dummyAddr("session-remote") }
func (c *Conn) SetDeadline(t time.Time) error      { return c.ws.SetReadDeadline(t) }
func (c *Conn) SetReadDeadline(t time.Time) error  { return c.ws.SetReadDeadline(t) }
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.ws.SetWriteDeadline(t) }

type dummyAddr string

func (d dummyAddr) Network() string { return "ws" }
func (d dummyAddr) String() string  { return string(d) }

func Pipe(ctx context.Context, a, b io.ReadWriteCloser) error {
	type copyResult struct {
		direction string
		bytes     int64
		err       error
	}
	errCh := make(chan copyResult, 2)
	go func() {
		n, err := io.Copy(a, b)
		errCh <- copyResult{direction: "a<-b", bytes: n, err: err}
	}()
	go func() {
		n, err := io.Copy(b, a)
		errCh <- copyResult{direction: "b<-a", bytes: n, err: err}
	}()

	closeBoth := func() {
		_ = a.Close()
		_ = b.Close()
	}
	resultErr := func(result copyResult) error {
		if result.err == nil {
			return nil
		}
		return fmt.Errorf("%s copied %d bytes: %w", result.direction, result.bytes, result.err)
	}
	select {
	case <-ctx.Done():
		closeBoth()
		return ctx.Err()
	case first := <-errCh:
		// EOF and connection-close errors commonly arrive immediately after a
		// TCP target writes its final response. Let the opposite direction drain
		// before closing both sides, otherwise unread peer data can turn Close
		// into a reset and discard the response frame that was just written.
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			closeBoth()
			return ctx.Err()
		case second := <-errCh:
			closeBoth()
			if err := resultErr(first); err != nil {
				return err
			}
			return resultErr(second)
		case <-timer.C:
			closeBoth()
			return resultErr(first)
		}
	}
}
