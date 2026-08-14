package app

import (
	"sort"
	"sync"

	"github.com/ops-bastion/ops/go/internal/gateway/core"
)

type Config = core.Config

type buildContext struct {
	cfg    Config
	server *core.Server
}

type registration func(*buildContext)

var featureRegistry = struct {
	sync.Mutex
	byName map[string]registration
}{byName: make(map[string]registration)}

func registerFeature(name string, wire registration) {
	if name == "" || wire == nil {
		panic("invalid gateway app registration")
	}
	featureRegistry.Lock()
	defer featureRegistry.Unlock()
	if _, exists := featureRegistry.byName[name]; exists {
		panic("duplicate gateway app registration: " + name)
	}
	featureRegistry.byName[name] = wire
}

// New is the Gateway's single concrete composition root.
func New(cfg Config) *core.Server {
	if cfg.AgentBinDir == "" {
		cfg.AgentBinDir = "bin"
	}
	s := core.New(cfg)
	ctx := &buildContext{cfg: cfg, server: s}

	featureRegistry.Lock()
	names := make([]string, 0, len(featureRegistry.byName))
	for name := range featureRegistry.byName {
		names = append(names, name)
	}
	sort.Strings(names)
	wires := make([]registration, 0, len(names))
	for _, name := range names {
		wires = append(wires, featureRegistry.byName[name])
	}
	featureRegistry.Unlock()
	for _, wire := range wires {
		wire(ctx)
	}
	return s
}
