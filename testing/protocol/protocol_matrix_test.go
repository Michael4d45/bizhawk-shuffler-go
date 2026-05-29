package protocol_test

import (
	"testing"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestCommandNamesRoundTrip(t *testing.T) {
	names := []protocol.CommandName{
		protocol.CmdHello, protocol.CmdAck, protocol.CmdPing, protocol.CmdSwap,
		protocol.CmdStateUpdate, protocol.CmdHelloAdmin,
	}
	for _, name := range names {
		cmd := protocol.Command{Cmd: name, ID: "t1"}
		raw, err := protocol.EncodeCommand(cmd)
		if err != nil {
			t.Fatal(err)
		}
		dec, err := protocol.DecodeCommand(raw)
		if err != nil || dec.Cmd != name {
			t.Fatalf("%s: %+v %v", name, dec, err)
		}
	}
}
