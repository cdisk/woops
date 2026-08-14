package socket

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/sessionws"
)

// runTCP dials target host:port and pipes binary frames with the session WSS.
func runTCP(ws *websocket.Conn, host string, port int) error {
	if host == "" {
		host = "127.0.0.1"
	}
	if port <= 0 {
		return fmt.Errorf("missing target port")
	}
	target := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", target, 15*time.Second)
	if err != nil {
		// Do not write ERROR text onto the data WS: gateway pipes bytes straight
		// to guacd/TCP, and a pre-pipe ReadMessage peek breaks RDP NLA.
		return err
	}
	defer conn.Close()
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}

	tunnel := sessionws.NewBinary(ws)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return sessionws.Pipe(ctx, tunnel, conn)
}
