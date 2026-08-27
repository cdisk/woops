package proxy

import (
	"context"
	"io"
	"log"
	"sync"

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
	// BridgeRaw is the independent YAML subtree for proxyBridge:.
	BridgeRaw []byte
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

	if len(deps.Raw) == 0 && len(deps.BridgeRaw) == 0 {
		log.Printf("proxy plugin: no proxy or proxyBridge section in agent.yaml - skipped")
		return
	}

	cfg, proxyParseErr := parse(deps.Raw)
	if proxyParseErr != nil {
		log.Printf("proxy plugin: invalid config: %v", proxyParseErr)
	}
	bridgeCfg, bridgeParseErr := parseBridge(deps.BridgeRaw)
	if bridgeParseErr != nil {
		log.Printf("proxy bridge plugin: invalid config: %v", bridgeParseErr)
	}
	proxyEnabled := proxyParseErr == nil && cfg.Enabled
	bridgeEnabled := bridgeParseErr == nil && bridgeCfg.Enabled
	if !proxyEnabled && !bridgeEnabled {
		log.Printf("proxy plugin: disabled (proxy.enabled=false, proxyBridge.enabled=false)")
		return
	}

	plog, closer, err := applog.New(deps.ConfigPath, "proxy.log")
	if err != nil {
		log.Printf("proxy plugin: not started - open proxy.log: %v", err)
		return
	}
	defer closer.Close()

	plog.Printf("proxy and bridge validating")
	if proxyEnabled {
		if err := cfg.validate(); err != nil {
			proxyEnabled = false
			plog.Printf("proxy FAIL reason=bad_config err=%q", err.Error())
			log.Printf("proxy plugin: not started - %v", err)
		}
	}
	if bridgeEnabled {
		if err := bridgeCfg.validate(); err != nil {
			bridgeEnabled = false
			plog.Printf("proxy bridge FAIL reason=bad_config err=%q", err.Error())
			log.Printf("proxy bridge plugin: not started - %v", err)
		}
	}
	if !proxyEnabled && !bridgeEnabled {
		return
	}
	upstream, err := tlsutil.ParseHTTPProxy(deps.UpstreamProxy)
	if err != nil {
		plog.Printf("proxy FAIL reason=bad_upstream err=%q", err.Error())
		log.Printf("proxy plugin: not started - bad gatewayProxy for upstream: %v", err)
		return
	}

	ops, err := parseOpsTarget(deps.Server)
	if err != nil {
		plog.Printf("proxy FAIL reason=bad_gateway")
		log.Printf("proxy plugin: not started - bad gateway")
		return
	}
	parents := newCarrierPool()
	defer parents.close()
	localProxyListen := cfg.Listen
	if localProxyListen == "" {
		localProxyListen = defaultListen
	}
	bridgePolicy := Config{AllowGlobal: bridgeCfg.AllowGlobal}
	bridgeHandler := &handler{
		cfg:          bridgePolicy,
		ops:          ops,
		upstream:     upstream,
		parent:       parents,
		bridgeEntry:  true,
		upstreamSelf: sameListenAddress(upstream, localProxyListen),
		log:          plog,
	}

	var wg sync.WaitGroup
	run := func(name string, fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil && ctx.Err() == nil {
				plog.Printf("%s FAIL reason=serve", name)
				log.Printf("%s plugin: stopped: %v", name, err)
			}
		}()
	}
	if bridgeCfg.Listen != "" && bridgeEnabled {
		run("proxy bridge", func() error {
			return serveBridgeListener(ctx, bridgeCfg, parents, plog)
		})
	}
	if proxyEnabled {
		run("proxy", func() error {
			return serveWithCarrier(ctx, cfg, deps.Server, upstream, parents, plog)
		})
	}
	if bridgeEnabled {
		for i := range bridgeCfg.Targets {
			target := bridgeCfg.Targets[i]
			wg.Add(1)
			go func() {
				defer wg.Done()
				runBridgeTarget(ctx, target, bridgeHandler, plog)
			}()
		}
	}
	<-ctx.Done()
	parents.close()
	wg.Wait()
}

func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}
