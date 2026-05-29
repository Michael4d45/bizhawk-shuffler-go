package protocol

import "testing"

func TestCodecRoundTripHello(t *testing.T) {
	cmd := Command{Cmd: CmdHello, ID: "1", Payload: map[string]any{"name": "p1"}}
	raw, err := EncodeCommand(cmd)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := DecodeCommand(raw)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Cmd != CmdHello || dec.ID != "1" {
		t.Fatalf("got %+v", dec)
	}
}
