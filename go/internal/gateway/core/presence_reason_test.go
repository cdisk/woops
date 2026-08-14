package core

import (
	"errors"
	"testing"

	"github.com/gorilla/websocket"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestClassifyControlClose(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "disconnected"},
		{"timeout", timeoutErr{}, "read_timeout"},
		{"normal close", &websocket.CloseError{Code: websocket.CloseNormalClosure}, "client_close"},
		{"going away", &websocket.CloseError{Code: websocket.CloseGoingAway}, "client_close"},
		{"abnormal", &websocket.CloseError{Code: websocket.CloseAbnormalClosure}, "abnormal_close"},
		{"generic", errors.New("something else"), "disconnected"},
		{"closed conn", errors.New("use of closed network connection"), "gateway_close"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyControlClose(tc.err); got != tc.want {
				t.Fatalf("classifyControlClose(%v)=%q want %q", tc.err, got, tc.want)
			}
		})
	}
}
