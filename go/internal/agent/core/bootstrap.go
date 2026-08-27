package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/host"
)

const maxRegisterResponseBytes = 1 << 20

type registerRequest struct {
	InstallCode  string `json:"installCode"`
	AssetID      string `json:"assetId,omitempty"`
	AgentVersion string `json:"agentVersion"`
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	PrivateIP    string `json:"privateIp"`
}

type registerResponse struct {
	AssetID    string `json:"assetId"`
	AgentToken string `json:"agentToken"`
}

type registerStatusError struct {
	statusCode int
	status     string
}

func (e *registerStatusError) Error() string {
	return "agent register: " + e.status
}

func (r *Runtime) bootstrap(ctx context.Context) error {
	if strings.TrimSpace(r.cfg.ConfigPath) == "" {
		return fmt.Errorf("config path unset")
	}
	dir := filepath.Dir(r.cfg.ConfigPath)
	codePath := filepath.Join(dir, "install-code")
	blockedCode := ""
	retry := r.retryBase

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		assetID, token := readCredentials(dir)
		r.cfg.AssetID, r.cfg.AgentToken = assetID, token
		code := strings.TrimSpace(readFile(codePath))

		if code == "" {
			blockedCode = ""
			retry = r.retryBase
			if assetID != "" && token != "" {
				return nil
			}
			if err := waitContext(ctx, r.bootstrapPoll); err != nil {
				return err
			}
			continue
		}
		if code == blockedCode {
			if err := waitContext(ctx, r.bootstrapPoll); err != nil {
				return err
			}
			continue
		}

		response, err := r.register(ctx, code, assetID)
		if err == nil {
			if err := persistCredentials(dir, response.AssetID, response.AgentToken); err != nil {
				log.Printf("agent register credentials: %v", err)
				if err := waitContext(ctx, jitter(retry)); err != nil {
					return err
				}
				retry = nextRetry(retry, r.retryMax)
				continue
			}
			for {
				if err := os.Remove(codePath); err == nil || errors.Is(err, os.ErrNotExist) {
					break
				} else {
					log.Printf("agent register install-code cleanup: %v", err)
				}
				if err := waitContext(ctx, jitter(retry)); err != nil {
					return err
				}
				retry = nextRetry(retry, r.retryMax)
			}
			if err := r.cfg.ReloadCredentials(); err != nil {
				return err
			}
			return nil
		}

		var statusErr *registerStatusError
		if errors.As(err, &statusErr) && statusErr.statusCode >= 400 && statusErr.statusCode < 500 {
			log.Printf("agent register rejected: %s", statusErr.status)
			blockedCode = code
			retry = r.retryBase
			continue
		}
		log.Printf("agent register retry: %v", err)
		if err := waitContext(ctx, jitter(retry)); err != nil {
			return err
		}
		retry = nextRetry(retry, r.retryMax)
	}
}

func (r *Runtime) register(ctx context.Context, installCode, assetID string) (registerResponse, error) {
	hostname, _ := os.Hostname()
	request := registerRequest{
		InstallCode:  installCode,
		AssetID:      strings.TrimSpace(assetID),
		AgentVersion: strings.TrimSpace(r.cfg.AgentVersion),
		Hostname:     hostname,
		OS:           detailedOS(),
		Arch:         runtime.GOARCH,
		PrivateIP:    CollectNetInfo().PrivateIPCSV(),
	}
	data, err := json.Marshal(request)
	if err != nil {
		return registerResponse{}, err
	}
	base, err := gatewayHTTPBase(r.cfg.Gateway)
	if err != nil {
		return registerResponse{}, err
	}
	endpoint := strings.TrimRight(base, "/") + "/api/agent/register"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return registerResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.http.Do(req)
	if err != nil {
		return registerResponse{}, fmt.Errorf("agent register request failed")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRegisterResponseBytes+1))
	if err != nil {
		return registerResponse{}, fmt.Errorf("agent register response read failed")
	}
	if len(body) > maxRegisterResponseBytes {
		return registerResponse{}, fmt.Errorf("agent register response too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return registerResponse{}, &registerStatusError{
			statusCode: resp.StatusCode,
			status:     resp.Status,
		}
	}
	var result registerResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return registerResponse{}, fmt.Errorf("agent register response invalid")
	}
	result.AssetID = strings.TrimSpace(result.AssetID)
	result.AgentToken = strings.TrimSpace(result.AgentToken)
	if result.AssetID == "" || result.AgentToken == "" {
		return registerResponse{}, fmt.Errorf("agent register response incomplete")
	}
	return result, nil
}

func detailedOS() string {
	info, err := host.Info()
	if err != nil || info == nil {
		return runtime.GOOS
	}
	parts := []string{info.Platform, info.PlatformVersion}
	if info.KernelVersion != "" {
		parts = append(parts, "kernel "+info.KernelVersion)
	}
	value := strings.TrimSpace(strings.Join(nonEmpty(parts), " "))
	if value == "" {
		return runtime.GOOS
	}
	return value
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func readCredentials(dir string) (string, string) {
	return strings.TrimSpace(readFile(filepath.Join(dir, "asset-id"))),
		strings.TrimSpace(readFile(filepath.Join(dir, "agent-token")))
}

func persistCredentials(dir, assetID, token string) error {
	if strings.TrimSpace(assetID) == "" || strings.TrimSpace(token) == "" {
		return fmt.Errorf("refusing empty credentials")
	}
	if err := atomicWriteCredential(filepath.Join(dir, "asset-id"), []byte(assetID+"\n")); err != nil {
		return fmt.Errorf("write asset-id: %w", err)
	}
	if err := atomicWriteCredential(filepath.Join(dir, "agent-token"), []byte(token+"\n")); err != nil {
		return fmt.Errorf("write agent-token: %w", err)
	}
	gotID, gotToken := readCredentials(dir)
	if gotID != assetID || gotToken != token {
		return fmt.Errorf("credential verification failed")
	}
	return nil
}

func waitContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		delay = time.Millisecond
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func nextRetry(current, maximum time.Duration) time.Duration {
	if current <= 0 {
		current = time.Second
	}
	if maximum <= 0 {
		maximum = 30 * time.Second
	}
	if current >= maximum/2 {
		return maximum
	}
	return current * 2
}

func jitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		delay = time.Second
	}
	// Add up to 25% jitter; no credential or installation-code material is used.
	return delay + time.Duration(rand.Int63n(int64(delay/4+1)))
}
