package serverhost

import (
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestEnqueueWSCommandSendsToChannel(t *testing.T) {
	ch := make(chan protocol.Command, 1)
	cmd := protocol.Command{Cmd: protocol.CmdPing, ID: "1"}
	if err := enqueueWSCommand(ch, cmd, time.Second, "test"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-ch:
		if got.ID != "1" {
			t.Fatalf("id %q", got.ID)
		}
	default:
		t.Fatal("expected command on channel")
	}
}

func TestEnqueueWSCommandClosedChannel(t *testing.T) {
	ch := make(chan protocol.Command)
	close(ch)
	err := enqueueWSCommand(ch, protocol.Command{Cmd: protocol.CmdPing}, time.Second, "test")
	if err == nil {
		t.Fatal("expected error for closed channel")
	}
}
