package proxy

import (
	"context"
	"io"
	"log"

	"github.com/ops-bastion/ops/go/internal/agent/infra/applog"
	"github.com/ops-bastion/ops/go/internal/tlsutil"
)

// Deps is the minimal read-only view the proxy plugin needs from the agent core.
type Deps struct {
	// Server is agent.yaml gateway (https://host:port), used when allowGlobal=false.
	Server string
	// UpstreamProxy is agent.yaml gatewayProxy; when set, inbound proxy chains
	// CONNECT/forward through it (C→B→A→Gateway).
	UpstreamProxy string
	// Raw is the YAML subtree for the proxy: key (may be nil/empty).
	Raw []byte
	// ConfigPath is agent.yaml path; used to place proxy.log next to woops-agent.log.
	ConfigPath string
}

// TryStart parses config and runs the HTTP forward proxy until ctx is cancelled.
// All failures are logged; this function never panics out to the caller.
// When enabled=true, opens rotating proxy.log; otherwise does not create the file.
func TryStart(ctx context.Context, deps Deps) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("proxy plugin: panic: %v - proxy not started", r)
		}
	}()

	if len(deps.Raw) == 0 {
		log.Printf("proxy plugin: no proxy section in agent.yaml - skipped")
		return
	}

	cfg, err := parse(deps.Raw)
	if err != nil {
		log.Printf("proxy plugin: not started - invalid config: %v", err)
		return
	}
	if !cfg.Enabled {
		log.Printf("proxy plugin: disabled (proxy.enabled=false)")
		return
	}

	plog, closer, err := applog.New(deps.ConfigPath, "proxy.log")
	if err != nil {
		log.Printf("proxy plugin: not started - open proxy.log: %v", err)
		return
	}
	defer closer.Close()

	plog.Printf("proxy validating")
	if err := cfg.validate(); err != nil {
		plog.Printf("proxy FAIL reason=bad_config err=%q", err.Error())
		log.Printf("proxy plugin: not started - %v", err)
		return
	}
	upstream, err := tlsutil.ParseHTTPProxy(deps.UpstreamProxy)
	if err != nil {
		plog.Printf("proxy FAIL reason=bad_upstream err=%q", err.Error())
		log.Printf("proxy plugin: not started - bad gatewayProxy for upstream: %v", err)
		return
	}
	if err := serve(ctx, cfg, deps.Server, upstream, plog); err != nil {
		plog.Printf("proxy FAIL reason=serve err=%q", err.Error())
		log.Printf("proxy plugin: not started - %v", err)
	}
}

func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}
