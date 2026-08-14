package app

import (
	"context"
	"sort"
	"sync"

	"github.com/ops-bastion/ops/go/internal/agent/core"
	"github.com/ops-bastion/ops/go/internal/agent/sessionreg"
)

type Config = core.Config

func LoadConfig(path string) (Config, error) {
	return core.LoadConfig(path)
}

type controlRegistration func(core.ControlSender, *sessionreg.ControlRegistry)

type buildContext struct {
	cfg          Config
	services     core.Services
	sessions     *sessionreg.Registry
	controls     []controlRegistration
	background   []core.BackgroundStarter
	onDisconnect []func()
}

type registration func(*buildContext)

var featureRegistry = struct {
	sync.Mutex
	byName map[string]registration
}{byName: make(map[string]registration)}

func registerFeature(name string, wire registration) {
	if name == "" || wire == nil {
		panic("invalid agent app registration")
	}
	featureRegistry.Lock()
	defer featureRegistry.Unlock()
	if _, exists := featureRegistry.byName[name]; exists {
		panic("duplicate agent app registration: " + name)
	}
	featureRegistry.byName[name] = wire
}

func (ctx *buildContext) registerSession(protocol, operationType string, run sessionreg.Runner) {
	ctx.sessions.Register(protocol, operationType, run)
}

func (ctx *buildContext) registerControl(wire controlRegistration) {
	ctx.controls = append(ctx.controls, wire)
}

func (ctx *buildContext) registerBackground(start func(context.Context)) {
	ctx.background = append(ctx.background, start)
}

func (ctx *buildContext) registerDisconnect(hook func()) {
	ctx.onDisconnect = append(ctx.onDisconnect, hook)
}

// New is the Agent composition root: core transports depend only on injected
// registries and lifecycle hooks, while concrete features self-register here.
func New(cfg Config) (*core.Runtime, error) {
	return core.New(cfg, func(services core.Services) core.Dependencies {
		ctx := &buildContext{cfg: cfg, services: services, sessions: sessionreg.New()}
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

		controls := func(send core.ControlSender) *sessionreg.ControlRegistry {
			reg := sessionreg.NewControl()
			for _, wire := range ctx.controls {
				wire(send, reg)
			}
			return reg
		}
		disconnect := func() {
			for _, hook := range ctx.onDisconnect {
				hook()
			}
		}

		return core.Dependencies{
			Sessions:            ctx.sessions,
			Controls:            controls,
			Background:          ctx.background,
			OnControlDisconnect: disconnect,
		}
	})
}
