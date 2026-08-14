package socket

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/datagram"
)

func TestRunUDPKeepsRepliesBoundToOriginalClient(t *testing.T) {
	target, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()

	serverWSCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			serverWSCh <- ws
		}
	}))
	defer server.Close()
	clientWS, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	serverWS := <-serverWSCh
	defer serverWS.Close()

	targetAddr := target.LocalAddr().(*net.UDPAddr)
	runDone := make(chan error, 1)
	go func() {
		runDone <- runUDP(clientWS, targetAddr.IP.String(), targetAddr.Port)
	}()

	clients := []struct {
		host    string
		port    int
		request string
		reply   string
	}{
		{"10.0.0.1", 31001, "request-a", "reply-a"},
		{"10.0.0.2", 31002, "request-b", "reply-b"},
	}
	for _, client := range clients {
		if err := serverWS.WriteMessage(websocket.BinaryMessage,
			datagram.Encode(client.host, client.port, []byte(client.request))); err != nil {
			t.Fatal(err)
		}
	}

	sourceByRequest := map[string]*net.UDPAddr{}
	buf := make([]byte, 128)
	_ = target.SetReadDeadline(time.Now().Add(3 * time.Second))
	for range clients {
		n, source, err := target.ReadFromUDP(buf)
		if err != nil {
			t.Fatal(err)
		}
		sourceByRequest[string(buf[:n])] = source
	}
	sourceA, sourceB := sourceByRequest["request-a"], sourceByRequest["request-b"]
	if sourceA == nil || sourceB == nil {
		t.Fatalf("target did not receive both requests: %#v", sourceByRequest)
	}
	if sourceA.String() == sourceB.String() {
		t.Fatal("different ingress clients unexpectedly shared one target UDP socket")
	}
	// Reply in reverse order to prove routing does not depend on the last sender.
	for i := len(clients) - 1; i >= 0; i-- {
		client := clients[i]
		if _, err := target.WriteToUDP([]byte(client.reply), sourceByRequest[client.request]); err != nil {
			t.Fatal(err)
		}
	}

	got := map[string]string{}
	_ = serverWS.SetReadDeadline(time.Now().Add(3 * time.Second))
	for range clients {
		_, frame, err := serverWS.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		host, port, payload, err := datagram.Decode(frame)
		if err != nil {
			t.Fatal(err)
		}
		got[string(payload)] = net.JoinHostPort(host, strconv.Itoa(port))
	}
	if got["reply-a"] != "10.0.0.1:31001" || got["reply-b"] != "10.0.0.2:31002" {
		t.Fatalf("replies crossed clients: %#v", got)
	}

	_ = serverWS.Close()
	select {
	case <-runDone:
	case <-time.After(3 * time.Second):
		t.Fatal("UDP tunnel did not stop")
	}
}
