package guac

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"testing"
)

func TestEncodeRoundTrip(t *testing.T) {
	raw := Encode("select", "rdp")
	op, args, err := ReadInstruction(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if op != "select" || len(args) != 1 || args[0] != "rdp" {
		t.Fatalf("got %s %v", op, args)
	}
}

func TestEncodeUsesUnicodeCharLengthNotBytes(t *testing.T) {
	// Guacamole length prefixes count code points; CJK must not use UTF-8 byte length.
	raw := Encode("name", "独断万股")
	if !bytes.HasPrefix(raw, []byte("4.name,4.独断万股;")) {
		t.Fatalf("want char-length prefix, got %q", raw)
	}
	op, args, err := ReadInstruction(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if op != "name" || len(args) != 1 || args[0] != "独断万股" {
		t.Fatalf("got %s %v", op, args)
	}
}

func TestReadInstructionUnicodeFromWire(t *testing.T) {
	// Simulate guacd: length is character count, value is UTF-8.
	raw := []byte("5.error,2.失败,3.519;")
	op, args, err := ReadInstruction(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if op != "error" || len(args) != 2 || args[0] != "失败" || args[1] != "519" {
		t.Fatalf("got %s %v", op, args)
	}
}

func TestEncodeConnectManyArgs(t *testing.T) {
	raw := Encode("connect", "h", "3389", "u", "p")
	op, args, err := ReadInstruction(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if op != "connect" || len(args) != 4 {
		t.Fatalf("got %s %v", op, args)
	}
}

func TestParseInstructionsAllowsSemicolonInsideElement(t *testing.T) {
	raw := append(Encode("msg", "a;b"), Encode("sync", "123")...)
	got, err := ParseInstructions(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Opcode != "msg" || got[0].Args[0] != "a;b" || got[1].Opcode != "sync" {
		t.Fatalf("unexpected instructions: %#v", got)
	}
}

func TestParseInstructionsRejectsPartialInstruction(t *testing.T) {
	raw := Encode("blob", "3", "abc")
	if _, err := ParseInstructions(raw[:len(raw)-1]); err == nil {
		t.Fatal("expected partial instruction error")
	}
}

func TestFramebufferInstructionOrderIsPreserved(t *testing.T) {
	raw := append(Encode("img", "0", "0", "0", "image/png", "0", "0"), Encode("blob", "0", "iVBORw0KGgo=")...)
	raw = append(raw, Encode("end", "0")...)
	raw = append(raw, Encode("sync", "99")...)
	got, err := ParseInstructions(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"img", "blob", "end", "sync"}
	if len(got) != len(want) {
		t.Fatalf("got %d instructions", len(got))
	}
	for i := range want {
		if got[i].Opcode != want[i] {
			t.Fatalf("instruction %d: got %q want %q", i, got[i].Opcode, want[i])
		}
	}
}

func TestVersionNegotiation(t *testing.T) {
	if got := negotiateVersion("VERSION_1_6_0"); got != "VERSION_1_5_0" {
		t.Fatalf("newer server: got %q", got)
	}
	if got := negotiateVersion("VERSION_1_1_0"); got != "VERSION_1_1_0" {
		t.Fatalf("older server: got %q", got)
	}
	if got := negotiateVersion("hostname"); got != "VERSION_1_0_0" {
		t.Fatalf("invalid version: got %q", got)
	}
}

func TestRDPClipboardParametersEnableBothDirections(t *testing.T) {
	params := ConnParams{Protocol: "rdp"}
	if got := paramValue("disable-copy", params); got != "false" {
		t.Fatalf("disable-copy = %q", got)
	}
	if got := paramValue("disable-paste", params); got != "false" {
		t.Fatalf("disable-paste = %q", got)
	}
	if got := paramValue("normalize-clipboard", params); got != "preserve" {
		t.Fatalf("normalize-clipboard = %q", got)
	}
}

func TestRDPColorDepthAndQualityDefaults(t *testing.T) {
	low := ConnParams{Protocol: "rdp"}
	if got := paramValue("color-depth", low); got != "16" {
		t.Fatalf("default color-depth = %q want 16", got)
	}
	if got := paramValue("enable-wallpaper", low); got != "false" {
		t.Fatalf("default wallpaper = %q want false", got)
	}
	hi := ConnParams{Protocol: "rdp", ColorDepth: 32, RdpQuality: "high"}
	if got := paramValue("color-depth", hi); got != "32" {
		t.Fatalf("color-depth = %q", got)
	}
	if got := paramValue("enable-font-smoothing", hi); got != "true" {
		t.Fatalf("high font-smoothing = %q", got)
	}
	med := ConnParams{Protocol: "rdp", ColorDepth: 24, RdpQuality: "medium"}
	if got := paramValue("enable-theming", med); got != "true" {
		t.Fatalf("medium theming = %q", got)
	}
	if got := paramValue("enable-wallpaper", med); got != "false" {
		t.Fatalf("medium wallpaper = %q", got)
	}
}

type oneByteReader struct{ io.Reader }

func (r oneByteReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.Reader.Read(p)
}

func TestReadInstructionAcrossPartialReads(t *testing.T) {
	br := bufio.NewReader(oneByteReader{bytes.NewReader(Encode("blob", "7", "a;b;c"))})
	op, args, err := ReadInstruction(br)
	if err != nil {
		t.Fatal(err)
	}
	if op != "blob" || len(args) != 2 || args[1] != "a;b;c" {
		t.Fatalf("got %q %#v", op, args)
	}
}

func TestConfiguredHandshakeConsumesReadyAndPreservesFollowingInstruction(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	serverErr := make(chan error, 1)
	go func() {
		br := bufio.NewReader(server)
		op, args, err := ReadInstruction(br)
		if err != nil || op != "select" || len(args) != 1 || args[0] != "rdp" {
			serverErr <- err
			return
		}
		if err := WriteInstruction(server, "args",
			"VERSION_1_5_0", "hostname", "port", "username", "password",
			"domain", "security", "ignore-cert",
		); err != nil {
			serverErr <- err
			return
		}
		wantOps := []string{"size", "audio", "video", "image", "timezone", "name", "connect"}
		for _, want := range wantOps {
			got, _, err := ReadInstruction(br)
			if err != nil || got != want {
				if err == nil {
					err = io.ErrUnexpectedEOF
				}
				serverErr <- err
				return
			}
		}
		payload := append(Encode("ready", "connection-123"), Encode("sync", "42")...)
		_, err = server.Write(payload)
		serverErr <- err
	}()

	conn, err := Handshake(client, ConnParams{
		Protocol: "rdp", Hostname: "bridge", Port: 1234,
		Username: "Administrator", Password: "secret", Domain: ".", Security: "nla",
	})
	if err != nil {
		t.Fatal(err)
	}
	if conn.ID != "connection-123" || conn.ProtocolVersion != "VERSION_1_5_0" {
		t.Fatalf("configured connection: %#v", conn)
	}
	op, args, err := ReadInstruction(conn.Reader)
	if err != nil || op != "sync" || len(args) != 1 || args[0] != "42" {
		t.Fatalf("post-ready instruction: %q %#v %v", op, args, err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}
