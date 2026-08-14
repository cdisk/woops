package sessionws

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

type pipeTestConn struct {
	read       io.Reader
	readGate   <-chan struct{}
	wrote      chan struct{}
	closed     chan struct{}
	closeOnce  sync.Once
	writeOnce  sync.Once
	writeBytes bytes.Buffer
}

func (c *pipeTestConn) Read(p []byte) (int, error) {
	if c.readGate != nil {
		<-c.readGate
	}
	if c.read == nil {
		return 0, io.EOF
	}
	return c.read.Read(p)
}

func (c *pipeTestConn) Write(p []byte) (int, error) {
	n, err := c.writeBytes.Write(p)
	c.writeOnce.Do(func() { close(c.wrote) })
	return n, err
}

func (c *pipeTestConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func TestPipeDrainsOppositeDirectionAfterCleanEOF(t *testing.T) {
	clientDone := make(chan struct{})
	client := &pipeTestConn{
		readGate: clientDone,
		wrote:    make(chan struct{}),
		closed:   make(chan struct{}),
	}
	target := &pipeTestConn{
		read:   bytes.NewBufferString("HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n"),
		wrote:  make(chan struct{}),
		closed: make(chan struct{}),
	}

	done := make(chan error, 1)
	go func() {
		done <- Pipe(context.Background(), client, target)
	}()

	select {
	case <-client.wrote:
	case <-time.After(time.Second):
		t.Fatal("response was not copied to client")
	}

	select {
	case <-client.closed:
		t.Fatal("client was closed before the opposite direction drained")
	case <-time.After(50 * time.Millisecond):
	}

	close(clientDone)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Pipe returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Pipe did not finish after both directions completed")
	}

	if got := client.writeBytes.String(); got != "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n" {
		t.Fatalf("unexpected copied response %q", got)
	}
}
