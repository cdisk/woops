//go:build windows

package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"golang.org/x/sys/windows/svc"
)

const windowsServiceName = "woops-agent"

func maybeRunService(cfgPath string) error {
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("detect windows service: %w", err)
	}
	if !isSvc {
		return runInteractive(cfgPath)
	}
	return svc.Run(windowsServiceName, &agentService{cfgPath: cfgPath})
}

type agentService struct {
	cfgPath string
}

func (m *agentService) Execute(_ []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- loadAndRun(m.cfgPath, ctx)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				err := <-errCh
				if err != nil && err != context.Canceled {
					log.Printf("woops-agent stop: %v", err)
					return true, 1
				}
				return false, 0
			default:
				log.Printf("unexpected service control: %v", c.Cmd)
			}
		case err := <-errCh:
			if err != nil && err != context.Canceled {
				log.Printf("woops-agent exited: %v", err)
				return true, 1
			}
			return false, 0
		}
	}
}

// Ensure cfg path stays absolute when SCM starts us from System32.
func absolutizeConfigPath(cfgPath string) string {
	if cfgPath == "" || filepath.IsAbs(cfgPath) {
		return cfgPath
	}
	abs, err := filepath.Abs(cfgPath)
	if err != nil {
		return cfgPath
	}
	return abs
}
