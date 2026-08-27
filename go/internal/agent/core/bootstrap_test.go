package core

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ops-bastion/ops/go/internal/agent/sessionreg"
	"github.com/ops-bastion/ops/go/internal/tlsutil"
)

func TestBootstrapCompleteCredentialsWithoutCodeSkipsRegister(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	cfg := testBootstrapConfig(t, server.URL, "", "asset-old", "token-old", "")
	rt := testRuntime(t, cfg, nil)
	if err := rt.bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatalf("registration calls = %d, want 0", calls.Load())
	}
}

func TestBootstrapTLSRegisterPersistsCredentialsAndDeletesCode(t *testing.T) {
	var got registerRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/register" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(registerResponse{AssetID: "asset-new", AgentToken: "token-new"})
	}))
	defer server.Close()
	pin := serverSPKIPin(t, server)

	cfg := testBootstrapConfig(t, server.URL, pin, "asset-old", "token-old", "install-secret")
	cfg.AgentVersion = "2608272122"
	rt := testRuntime(t, cfg, nil)
	if err := rt.bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got.InstallCode != "install-secret" || got.AssetID != "asset-old" {
		t.Fatalf("registration identity fields = %+v", got)
	}
	if got.AgentVersion != cfg.AgentVersion || got.Hostname == "" || got.OS == "" ||
		got.Arch != runtime.GOARCH {
		t.Fatalf("registration host fields = %+v", got)
	}
	dir := filepath.Dir(cfg.ConfigPath)
	assetID, token := readCredentials(dir)
	if assetID != "asset-new" || token != "token-new" {
		t.Fatalf("persisted asset=%q token=%q", assetID, token)
	}
	if _, err := os.Stat(filepath.Join(dir, "install-code")); !os.IsNotExist(err) {
		t.Fatalf("install-code still exists: %v", err)
	}
	if rt.cfg.AssetID != "asset-new" || rt.cfg.AgentToken != "token-new" {
		t.Fatalf("runtime credentials not refreshed: %+v", rt.cfg)
	}
}

func TestBootstrapFourXXBlocksSameCodeAndNewCodeResumes(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	firstCall := make(chan struct{})
	var firstOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request registerRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		mu.Lock()
		calls[request.InstallCode]++
		mu.Unlock()
		if request.InstallCode == "bad-code" {
			firstOnce.Do(func() { close(firstCall) })
			http.Error(w, "do not expose this response", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(registerResponse{AssetID: "asset-new", AgentToken: "token-new"})
	}))
	defer server.Close()

	cfg := testBootstrapConfig(t, server.URL, "", "", "", "bad-code")
	rt := testRuntime(t, cfg, nil)
	rt.bootstrapPoll = 15 * time.Millisecond
	done := make(chan error, 1)
	go func() { done <- rt.bootstrap(context.Background()) }()
	select {
	case <-firstCall:
	case <-time.After(time.Second):
		t.Fatal("first registration did not arrive")
	}
	time.Sleep(70 * time.Millisecond)
	mu.Lock()
	badCalls := calls["bad-code"]
	mu.Unlock()
	if badCalls != 1 {
		t.Fatalf("same rejected code called %d times", badCalls)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(cfg.ConfigPath), "install-code"), []byte("good-code\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("new install code did not resume registration")
	}
	mu.Lock()
	goodCalls := calls["good-code"]
	mu.Unlock()
	if goodCalls != 1 {
		t.Fatalf("new code called %d times", goodCalls)
	}
}

func TestBootstrapContextCancellation(t *testing.T) {
	cfg := testBootstrapConfig(t, "http://127.0.0.1:1", "", "", "", "")
	rt := testRuntime(t, cfg, nil)
	rt.bootstrapPoll = time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- rt.bootstrap(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("bootstrap did not cancel promptly")
	}
}

func TestRuntimeStartsPreAuthBeforePostAuth(t *testing.T) {
	cfg := testBootstrapConfig(t, "http://127.0.0.1:1", "", "asset", "token", "")
	var mu sync.Mutex
	var order []string
	start := func(name string) BackgroundStarter {
		return func(ctx context.Context) {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			<-ctx.Done()
		}
	}
	rt := testRuntime(t, cfg, func(Services) Dependencies {
		return Dependencies{
			Sessions:          sessionreg.New(),
			PreAuthBackground: []BackgroundStarter{start("pre")},
			Background:        []BackgroundStarter{start("post")},
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- rt.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		count := len(order)
		mu.Unlock()
		if count >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runtime did not stop")
	}
	mu.Lock()
	defer mu.Unlock()
	if fmt.Sprint(order) != "[pre post]" {
		t.Fatalf("background order = %v", order)
	}
}

func testBootstrapConfig(t *testing.T, gateway, pin, assetID, token, code string) Config {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "agent.yaml")
	content := "gateway: " + gateway + "\n"
	if pin != "" {
		content += "gatewayTlsSpkiSha256: " + pin + "\n"
	}
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"asset-id": assetID, "agent-token": token, "install-code": code,
	} {
		if value != "" {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(value+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func testRuntime(t *testing.T, cfg Config, compose Composer) *Runtime {
	t.Helper()
	if compose == nil {
		compose = func(Services) Dependencies {
			return Dependencies{Sessions: sessionreg.New()}
		}
	}
	rt, err := New(cfg, compose)
	if err != nil {
		t.Fatal(err)
	}
	rt.bootstrapPoll = 20 * time.Millisecond
	rt.retryBase = 10 * time.Millisecond
	rt.retryMax = 40 * time.Millisecond
	return rt
}

func serverSPKIPin(t *testing.T, server *httptest.Server) string {
	t.Helper()
	if len(server.TLS.Certificates) == 0 || len(server.TLS.Certificates[0].Certificate) == 0 {
		t.Fatal("test server certificate unavailable")
	}
	cert, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return tlsutil.SPKIPinHex(cert)
}
