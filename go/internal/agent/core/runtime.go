package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/agent/sessionreg"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	"github.com/ops-bastion/ops/go/internal/tlsutil"
	"github.com/ops-bastion/ops/go/internal/wsutil"
)

type ControlSender func(typ, requestID string, payload any) error
type ControlFactory func(send ControlSender) *sessionreg.ControlRegistry
type BackgroundStarter func(ctx context.Context)

// Services are core-owned transports exposed to the application composition root.
type Services struct {
	Config        Config
	Dialer        *websocket.Dialer
	DialSessionWS func(sessionID, ticket string) (*websocket.Conn, error)
	PostJSON      func(path string, body, out any) error
}

// Dependencies are supplied by agent/app. Core never imports concrete modules.
type Dependencies struct {
	Sessions            *sessionreg.Registry
	Controls            ControlFactory
	PreAuthBackground   []BackgroundStarter
	Background          []BackgroundStarter
	OnControlDisconnect func()
}

type Composer func(services Services) Dependencies

type Runtime struct {
	cfg    Config
	dialer *websocket.Dialer
	http   *http.Client
	deps   Dependencies

	bootstrapPoll time.Duration
	retryBase     time.Duration
	retryMax      time.Duration
}

func New(cfg Config, compose Composer) (*Runtime, error) {
	d, err := NewWSDialer(cfg.Gateway, cfg.GatewayTLSSpkiSHA256, cfg.GatewayProxy)
	if err != nil {
		return nil, err
	}
	httpClient, err := tlsutil.HTTPClient(tlsutil.DialOptions{
		Pin:   cfg.GatewayTLSSpkiSHA256,
		Proxy: cfg.GatewayProxy,
	}, 20*time.Second)
	if err != nil {
		return nil, err
	}
	r := &Runtime{
		cfg:           cfg,
		dialer:        d,
		http:          httpClient,
		bootstrapPoll: 30 * time.Second,
		retryBase:     time.Second,
		retryMax:      30 * time.Second,
	}
	if compose == nil {
		return nil, fmt.Errorf("agent core composer required")
	}
	r.deps = compose(Services{
		Config:        cfg,
		Dialer:        d,
		DialSessionWS: r.dialSessionWS,
		PostJSON:      r.postJSON,
	})
	if r.deps.Sessions == nil {
		return nil, fmt.Errorf("agent session registry required")
	}
	return r, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	startBackgrounds(ctx, r.deps.PreAuthBackground)
	if err := r.bootstrap(ctx); err != nil {
		return err
	}
	startBackgrounds(ctx, r.deps.Background)
	return r.runControlLoop(ctx)
}

func startBackgrounds(ctx context.Context, starters []BackgroundStarter) {
	for _, starter := range starters {
		if starter == nil {
			continue
		}
		started := make(chan struct{})
		go func(start BackgroundStarter) {
			close(started)
			start(ctx)
		}(starter)
		<-started
	}
}

func (r *Runtime) runControlLoop(ctx context.Context) error {
	retry := r.retryBase
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		connected, err := r.runOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if connected {
				log.Printf("control ended err=%v", err)
				retry = r.retryBase
			} else {
				log.Printf("control connect retry: %v", err)
				retry = nextRetry(retry, r.retryMax)
			}
		} else if connected {
			retry = r.retryBase
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(jitter(retry)):
		}
	}
}

func (r *Runtime) runOnce(ctx context.Context) (connected bool, err error) {
	if err := r.cfg.ReloadCredentials(); err != nil {
		log.Printf("reload credentials: %v", err)
	}
	header := http.Header{}
	header.Set("X-Asset-Id", r.cfg.AssetID)
	header.Set("X-Agent-Token", r.cfg.AgentToken)
	ws, _, err := r.dialer.DialContext(ctx, r.cfg.ControlWS, header)
	if err != nil {
		return false, err
	}
	connected = true
	defer func() {
		if r.deps.OnControlDisconnect != nil {
			r.deps.OnControlDisconnect()
		}
		_ = ws.Close()
	}()
	log.Printf("control connected")

	// Unblock ReadMessage on SIGTERM / service stop (deadline alone can wait ~90s).
	go func() {
		<-ctx.Done()
		_ = ws.Close()
	}()

	// Keepalive: Gateway pings every 25s. Refresh deadline on Ping (inbound) and
	// Pong (if we ever ping). Without PingHandler, Agent hit i/o timeout at ~90s.
	wsutil.EnableReadDeadline(ws, wsutil.DefaultIdle)

	var writeMu sync.Mutex
	send := func(msg []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return ws.WriteMessage(websocket.TextMessage, msg)
	}
	sendNetInfo := func() {
		info := CollectNetInfo()
		msg, err := control.Marshal("netinfo", "", control.NetInfoPayload{
			PrivateIP: info.PrivateIPCSV(),
		})
		if err != nil {
			log.Printf("netinfo marshal err=%v", err)
			return
		}
		if err := send(msg); err != nil {
			log.Printf("netinfo send err=%v", err)
			return
		}
		// Success is silent: periodic private-IP refresh must not flood logs.
	}
	sendNetInfo()
	// Refresh private IPs periodically; public/source IP is set by Gateway on connect.
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				sendNetInfo()
			}
		}
	}()

	ctrl := sessionreg.NewControl()
	if r.deps.Controls != nil {
		if injected := r.deps.Controls(func(typ, requestID string, payload any) error {
			msg, err := control.Marshal(typ, requestID, payload)
			if err != nil {
				return err
			}
			return send(msg)
		}); injected != nil {
			ctrl = injected
		}
	}
	ctrl.Register("open_session", func(env control.Envelope) error {
		payload, err := control.UnmarshalPayload[control.OpenSessionPayload](env)
		if err == nil {
			go r.handleOpenSession(payload)
		}
		return nil
	})
	for {
		if err := ctx.Err(); err != nil {
			return connected, err
		}
		_, data, err := ws.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return connected, ctx.Err()
			}
			return connected, err
		}
		_ = ws.SetReadDeadline(time.Now().Add(wsutil.DefaultIdle))
		var env control.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		switch env.Type {
		case "hello_ack", "pong":
			continue
		}
		if _, err := ctrl.Handle(env); err != nil {
			log.Printf("control %s err=%v", env.Type, err)
		}
	}
}
