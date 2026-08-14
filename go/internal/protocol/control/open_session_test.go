package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildOpenSessionNestedOnly(t *testing.T) {
	p, err := BuildOpenSession("sid", "filetransfer", "tkt", FileTransferParams{
		TransferID: "11111111-1111-1111-1111-111111111111",
		Direction:  "upload",
		Path:       "/tmp/a",
		Size:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Params) == 0 {
		t.Fatal("expected nested params")
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if _, ok := wire["transferId"]; ok {
		t.Fatalf("legacy flat transferId present: %s", raw)
	}
	if _, ok := wire["direction"]; ok {
		t.Fatalf("legacy flat direction present: %s", raw)
	}
	if _, ok := wire["params"]; !ok {
		t.Fatalf("missing nested params: %s", raw)
	}
	var nested FileTransferParams
	if err := json.Unmarshal(p.Params, &nested); err != nil {
		t.Fatal(err)
	}
	if nested.Path != "/tmp/a" || nested.Size != 10 || nested.Direction != "upload" {
		t.Fatalf("nested=%+v", nested)
	}
}

func TestUnmarshalOpenParams(t *testing.T) {
	nested, _ := json.Marshal(ShellParams{ShellKind: "bash"})
	p := OpenSessionPayload{
		SessionID: "sid",
		Protocol:  "SHELL",
		Ticket:    "t",
		Params:    nested,
	}
	params, err := UnmarshalOpenParams[ShellParams](p)
	if err != nil {
		t.Fatal(err)
	}
	if params.ShellKind != "bash" {
		t.Fatalf("want bash, got %+v", params)
	}
	if NormalizeOpenSession(p).Protocol != "shell" {
		t.Fatalf("protocol normalize failed: %q", NormalizeOpenSession(p).Protocol)
	}
}

func TestGoldenOpenSessionRoundTrip(t *testing.T) {
	dir := filepath.Join("testdata", "open_session")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			var p OpenSessionPayload
			if err := json.Unmarshal(raw, &p); err != nil {
				t.Fatal(err)
			}
			var wire map[string]any
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			for _, flat := range []string{"shellKind", "transferId", "direction", "path", "size", "fingerprint", "abort", "targetHost", "targetPort"} {
				if _, ok := wire[flat]; ok {
					t.Fatalf("legacy flat field %q must not appear at open_session root in %s", flat, name)
				}
			}
			norm := NormalizeOpenSession(p)
			if norm.SessionID == "" || norm.Protocol == "" || norm.Ticket == "" {
				t.Fatalf("incomplete common fields: %+v", norm)
			}
			switch norm.Protocol {
			case "shell":
				sp, err := UnmarshalOpenParams[ShellParams](norm)
				if err != nil {
					t.Fatal(err)
				}
				if sp.ShellKind == "" {
					t.Fatalf("shell params empty: %+v", sp)
				}
			case "filetransfer":
				fp, err := UnmarshalOpenParams[FileTransferParams](norm)
				if err != nil {
					t.Fatal(err)
				}
				if fp.Path == "" {
					t.Fatalf("filetransfer params empty: %+v", fp)
				}
			case "rdp", "vnc", "tcp", "udp":
				tp, err := UnmarshalOpenParams[TunnelParams](norm)
				if err != nil {
					t.Fatal(err)
				}
				if tp.TargetPort <= 0 {
					t.Fatalf("tunnel params empty: %+v", tp)
				}
			case "filemanager", "exec":
				if strings.TrimSpace(string(norm.Params)) != "" && string(norm.Params) != "null" {
					t.Fatalf("%s should have empty params, got %s", norm.Protocol, norm.Params)
				}
			}
		})
	}
}
