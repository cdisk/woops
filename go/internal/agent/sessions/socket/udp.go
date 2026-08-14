package socket

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/datagram"
)

// runUDP dials a UDP target and bridges framed datagrams with the session WSS.
// Framing uses the shared protocol/datagram codec.
func runUDP(ws *websocket.Conn, host string, port int) error {
	if host == "" {
		host = "127.0.0.1"
	}
	if port <= 0 {
		return fmt.Errorf("missing udp target port")
	}
	target := net.JoinHostPort(host, strconv.Itoa(port))
	raddr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		return err
	}
	type peer struct {
		conn     *net.UDPConn
		done     chan struct{}
		once     sync.Once
		lastSeen time.Time
	}
	closePeer := func(p *peer) {
		p.once.Do(func() {
			close(p.done)
			_ = p.conn.Close()
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var peersMu sync.Mutex
	var writeMu sync.Mutex
	peers := make(map[string]*peer)
	defer func() {
		peersMu.Lock()
		defer peersMu.Unlock()
		for key, p := range peers {
			closePeer(p)
			delete(peers, key)
		}
	}()
	errCh := make(chan error, 1)
	fail := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	go func() {
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				fail(err)
				return
			}
			clientHost, clientPort, payload, err := datagram.Decode(data)
			if err != nil {
				continue
			}
			clientAddr := net.JoinHostPort(clientHost, strconv.Itoa(clientPort))
			peersMu.Lock()
			p := peers[clientAddr]
			if p == nil {
				conn, dialErr := net.DialUDP("udp", nil, raddr)
				if dialErr != nil {
					peersMu.Unlock()
					continue
				}
				p = &peer{conn: conn, done: make(chan struct{}), lastSeen: time.Now()}
				peers[clientAddr] = p
				go func(key, replyHost string, replyPort int, current *peer) {
					buf := make([]byte, 64*1024)
					for {
						n, readErr := current.conn.Read(buf)
						if readErr != nil {
							select {
							case <-current.done:
							default:
								peersMu.Lock()
								if peers[key] == current {
									delete(peers, key)
								}
								peersMu.Unlock()
								closePeer(current)
							}
							return
						}
						peersMu.Lock()
						if peers[key] == current {
							current.lastSeen = time.Now()
						}
						peersMu.Unlock()
						frame := datagram.Encode(replyHost, replyPort, buf[:n])
						writeMu.Lock()
						writeErr := ws.WriteMessage(websocket.BinaryMessage, frame)
						writeMu.Unlock()
						if writeErr != nil {
							fail(writeErr)
							return
						}
					}
				}(clientAddr, clientHost, clientPort, p)
			} else {
				p.lastSeen = time.Now()
			}
			conn := p.conn
			peersMu.Unlock()
			if _, err := conn.Write(payload); err != nil {
				peersMu.Lock()
				if peers[clientAddr] == p {
					delete(peers, clientAddr)
				}
				peersMu.Unlock()
				closePeer(p)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-time.After(30 * time.Second):
				cutoff := now.Add(-2 * time.Minute)
				peersMu.Lock()
				for key, p := range peers {
					if p.lastSeen.Before(cutoff) {
						delete(peers, key)
						closePeer(p)
					}
				}
				peersMu.Unlock()
			}
		}
	}()

	err = <-errCh
	if err != nil && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		return err
	}
	return nil
}
