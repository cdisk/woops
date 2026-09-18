package install

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMergeAgentYAMLFresh(t *testing.T) {
	got := MergeAgentYAML(nil, "https://gw.example:9200", "aabb")
	if !bytes.Contains(got, []byte(`gateway: "https://gw.example:9200"`)) {
		t.Fatalf("missing gateway: %s", got)
	}
	if !bytes.Contains(got, []byte(`gatewayTlsSpkiSha256: "aabb"`)) {
		t.Fatalf("missing pin: %s", got)
	}
	if !bytes.Contains(got, []byte("proxy:")) {
		t.Fatal("expected template proxy comments")
	}
}

func TestMergeAgentYAMLPreservesLocal(t *testing.T) {
	old := []byte("" +
		"gateway: \"https://old.example:9200\"\n" +
		"gatewayTlsSpkiSha256: \"0000\"\n" +
		"gatewayProxy: \"http://user:p%40ss@10.0.0.1:3128\"\n" +
		"\n" +
		"proxyBridge:\n" +
		"  enabled: true\n" +
		"  gateway: \"http://10.0.0.9:9100\"\n" +
		"\n" +
		"metrics:\n" +
		"  enabled: false\n")
	got := MergeAgentYAML(old, "https://woops.dev.h2v.top:9200", "deadbeef")
	want := []byte("" +
		"gateway: \"https://woops.dev.h2v.top:9200\"\n" +
		"gatewayTlsSpkiSha256: \"deadbeef\"\n" +
		"gatewayProxy: \"http://user:p%40ss@10.0.0.1:3128\"\n" +
		"\n" +
		"proxyBridge:\n" +
		"  enabled: true\n" +
		"  gateway: \"http://10.0.0.9:9100\"\n" +
		"\n" +
		"metrics:\n" +
		"  enabled: false\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeAgentYAMLBOMAndCRLF(t *testing.T) {
	body := "gateway: \"https://old.example:9200\"\r\n" +
		"gatewayTlsSpkiSha256: \"0000\"\r\n" +
		"proxyBridge:\r\n" +
		"  gateway: \"http://nested\"\r\n"
	old := append([]byte{0xEF, 0xBB, 0xBF}, []byte(body)...)
	got := MergeAgentYAML(old, "https://new.example:9200", "pin1")
	if !bytes.HasPrefix(got, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("BOM lost")
	}
	if !bytes.Contains(got, []byte("\r\n")) {
		t.Fatal("CRLF lost")
	}
	if !bytes.Contains(got, []byte(`gateway: "https://new.example:9200"`)) {
		t.Fatal("gateway not refreshed")
	}
	if !bytes.Contains(got, []byte("  gateway: \"http://nested\"")) {
		t.Fatal("nested gateway must survive")
	}
	// Must not leave the old top-level gateway line.
	if bytes.Contains(got, []byte("old.example")) {
		t.Fatal("stale gateway survived")
	}
}

func TestMergeAgentYAMLMissingPin(t *testing.T) {
	old := []byte("gateway: \"https://old.example:9200\"\nproxyBridge:\n  enabled: true\n")
	got := MergeAgentYAML(old, "https://new.example:9200", "pin1")
	if !bytes.Contains(got, []byte(`gatewayTlsSpkiSha256: "pin1"`)) {
		t.Fatalf("pin not inserted: %s", got)
	}
	if !bytes.Contains(got, []byte("proxyBridge:")) {
		t.Fatal("proxyBridge lost")
	}
}

func TestMergeAgentYAMLNoTrailingNewline(t *testing.T) {
	old := []byte(`gateway: "https://old.example:9200"`)
	got := MergeAgentYAML(old, "https://new.example:9200", "p")
	if !bytes.Contains(got, []byte(`gateway: "https://new.example:9200"`)) {
		t.Fatal("gateway not refreshed")
	}
	if !bytes.Contains(got, []byte(`gatewayTlsSpkiSha256: "p"`)) {
		t.Fatal("pin missing")
	}
}

func TestUpsertQuotedKey(t *testing.T) {
	doc := []byte("gateway: \"https://g\"\ngatewayTlsSpkiSha256: \"p\"\n")
	got := UpsertQuotedKey(doc, "gatewayProxy", `http://u:p"x@10.0.0.1:3128`)
	if !bytes.Contains(got, []byte(`gatewayProxy: "http://u:p\"x@10.0.0.1:3128"`)) {
		t.Fatalf("unexpected: %s", got)
	}
}

func TestWriteAgentYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte("gateway: \"https://old\"\nproxyBridge:\n  enabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("https_proxy", "http://proxy:3128")
	if err := WriteAgentYAML(path, "https://new", "pin"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`gateway: "https://new"`)) {
		t.Fatalf("%s", got)
	}
	if !bytes.Contains(got, []byte(`gatewayProxy: "http://proxy:3128"`)) {
		t.Fatalf("proxy not written: %s", got)
	}
	if !bytes.Contains(got, []byte("proxyBridge:")) {
		t.Fatal("proxyBridge lost")
	}
}
