package filetransfer

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestEncodeDecodeChunkRoundTrip(t *testing.T) {
	payload := bytes.Repeat([]byte("abc"), 1000)
	frame, err := EncodeChunk(42, payload)
	if err != nil {
		t.Fatal(err)
	}
	off, data, err := DecodeChunk(frame)
	if err != nil {
		t.Fatal(err)
	}
	if off != 42 {
		t.Fatalf("offset=%d", off)
	}
	if !bytes.Equal(data, payload) {
		t.Fatal("payload mismatch")
	}
	o, n, ok := PeekHeader(frame)
	if !ok || o != 42 || n != len(payload) {
		t.Fatalf("peek = %d %d %v", o, n, ok)
	}
}

func TestDecodeChunkBadChecksum(t *testing.T) {
	frame, err := EncodeChunk(0, []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	frame[20] ^= 0xff
	if _, _, err := DecodeChunk(frame); err != ErrChecksum {
		t.Fatalf("want ErrChecksum, got %v", err)
	}
}

func TestEncodeChunkTooLarge(t *testing.T) {
	big := make([]byte, MaxChunk+1)
	if _, err := EncodeChunk(0, big); err == nil {
		t.Fatal("expected error")
	}
}

func TestControlJSON(t *testing.T) {
	raw, err := EncodeControl(Control{Type: CtrlAck, Offset: 10, OK: true})
	if err != nil {
		t.Fatal(err)
	}
	c, err := DecodeControl(raw)
	if err != nil || c.Type != CtrlAck || c.Offset != 10 || !c.OK {
		t.Fatalf("decoded=%+v err=%v", c, err)
	}
	_ = sha256.Sum256(nil)
}
