package proxy

import (
	"context"
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
