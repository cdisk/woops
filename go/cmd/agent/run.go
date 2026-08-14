package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ops-bastion/ops/go/internal/agent/app"
	"github.com/ops-bastion/ops/go/internal/agent/infra/applog"
)

func runInteractive(cfgPath string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return loadAndRun(cfgPath, ctx)
}

func resolveConfigPath(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	if v := os.Getenv("OPS_AGENT_CONFIG"); v != "" {
		return v
	}
	return "agent.yaml"
}

func loadAndRun(cfgPath string, ctx context.Context) error {
	if closer, err := applog.SetupDefault(cfgPath); err != nil {
		log.Printf("applog: %v (stderr only)", err)
	} else if closer != nil {
		defer closer.Close()
	}

	cfg, err := app.LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	log.Printf("woops-agent %s starting asset=%s", Version, cfg.AssetID)
	rt, err := app.New(cfg)
	if err != nil {
		return err
	}
	if err := rt.Run(ctx); err != nil && err != context.Canceled {
		return err
	}
	return nil
}
