package control

import (
	"encoding/json"
	"testing"
)

func TestOpenSessionFileTransferMarshal(t *testing.T) {
	p, err := BuildOpenSession("sid", "filetransfer", "t", FileTransferParams{
		TransferID:  "11111111-1111-1111-1111-111111111111",
		Direction:   "upload",
		Path:        "/tmp/a",
		Size:        10,
		Fingerprint: "fp",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Marshal("open_session", "sid", p)
	if err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalPayload[OpenSessionPayload](env)
	if err != nil {
		t.Fatal(err)
	}
	params, err := UnmarshalOpenParams[FileTransferParams](got)
	if err != nil {
		t.Fatal(err)
	}
	if got.Protocol != "filetransfer" || params.TransferID == "" || params.Direction != "upload" || params.Size != 10 {
		t.Fatalf("payload=%+v params=%+v", got, params)
	}
}

func TestAbortTransferMarshal(t *testing.T) {
	raw, err := Marshal("abort_transfer", "r1", AbortTransferPayload{
		TransferID: "tid", Path: "/x",
	})
	if err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env.Type != "abort_transfer" {
		t.Fatalf("type=%s", env.Type)
	}
}
