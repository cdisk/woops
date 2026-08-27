package proxy

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ops-bastion/ops/go/internal/agent/infra/applog"
)

func TestTryStartCreatesProxyLogOnlyWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	// Force applog into temp dir on all platforms.
	// applog.dirOverride is package-private; set via New's ResolveDir by
	// temporarily using the test hook through a local override helper.
	setTestLogDir(t, dir)

	cfgPath := filepath.Join(dir, "agent.yaml")
	_ = os.WriteFile(cfgPath, []byte("gateway: https://x\n"), 0o644)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	TryStart(ctx, Deps{
		Server:     "10.0.0.1:9200",
		ConfigPath: cfgPath,
		Raw:        []byte("enabled: false\nusername: u\npassword: p\n"),
	})
	if _, err := os.Stat(filepath.Join(dir, "proxy.log")); !os.IsNotExist(err) {
		t.Fatalf("proxy.log should not exist when disabled: err=%v", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel2()
	// Listen on ephemeral - bind will succeed briefly then ctx ends.
	TryStart(ctx2, Deps{
		Server:     "10.0.0.1:9200",
		ConfigPath: cfgPath,
		Raw: []byte(`
enabled: true
listen: "127.0.0.1:0"
username: u
password: p
allowGlobal: true
`),
	})
	b, err := os.ReadFile(filepath.Join(dir, "proxy.log"))
	if err != nil {
		t.Fatalf("expected proxy.log: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "proxy listening") && !strings.Contains(s, "proxy validating") {
		t.Fatalf("proxy.log content unexpected: %q", s)
	}
	if strings.Contains(s, "password") || strings.Contains(s, "Proxy-Authorization") {
		t.Fatalf("sensitive data in proxy.log: %q", s)
	}
}

// setTestLogDir redirects applog.ResolveDir via the test-only override.
func setTestLogDir(t *testing.T, dir string) {
	t.Helper()
	applog.SetDirOverrideForTest(dir)
	t.Cleanup(func() { applog.SetDirOverrideForTest("") })
}

func TestMalformedBridgeDoesNotStopOrdinaryProxy(t *testing.T) {
	dir := t.TempDir()
	setTestLogDir(t, dir)
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := reserved.Addr().String()
	_ = reserved.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		TryStart(ctx, Deps{
			Server:     "https://gw.example:9200",
			ConfigPath: filepath.Join(dir, "agent.yaml"),
			Raw: []byte(`
enabled: true
listen: "` + addr + `"
username: u
password: p
`),
			BridgeRaw: []byte("enabled: [invalid"),
		})
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatal("ordinary proxy did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("proxy did not stop")
	}
}
