package datagram

import "testing"

func TestEncodeDecodeRoundTrip(t *testing.T) {
	frame := Encode("192.168.1.1", 53, []byte{1, 2, 3})
	host, port, payload, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if host != "192.168.1.1" || port != 53 || len(payload) != 3 {
		t.Fatalf("%s:%d %v", host, port, payload)
	}
	if _, _, _, err := Decode(frame[:4]); err == nil {
		t.Fatal("expected short frame error")
	}
}

func TestEncodeDecodeStringPayload(t *testing.T) {
	frame := Encode("10.1.2.3", 45678, []byte("hello"))
	host, port, payload, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if host != "10.1.2.3" || port != 45678 || string(payload) != "hello" {
		t.Fatalf("%s:%d %q", host, port, payload)
	}
}
