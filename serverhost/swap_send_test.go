package serverhost

import (
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestSendSwapSkipsWhenNotReady(t *testing.T) {
	chdirToTemp(t)
	s := New()
	registerPlayerWSClient(s, "bob")
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Players["bob"] = protocol.Player{
			Name: "bob", Game: "a.zip", Connected: true, BizhawkReady: false,
		}
	})

	s.sendSwap(protocol.Player{Name: "bob", Game: "a.zip"}, SwapSendOptions{})
	if n := s.PendingInstanceCount(); n != 0 {
		t.Fatalf("pendingInstancecount %d", n)
	}
}

func TestSendSwapSkipsWhenSwapInFlight(t *testing.T) {
	chdirToTemp(t)
	s := New()
	registerPlayerWSClient(s, "bob")
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Players["bob"] = protocol.Player{
			Name: "bob", Game: "a.zip", Connected: true, BizhawkReady: true,
		}
	})
	s.withLock(func() { s.swapInFlight["bob"] = struct{}{} })

	s.sendSwap(s.currentPlayer("bob"), SwapSendOptions{})
	time.Sleep(50 * time.Millisecond)
	select {
	case <-s.playerClients["bob"].sendCh:
		t.Fatal("unexpected swap command while in flight")
	default:
	}
}

func TestSendSwapSkipsUnchangedTarget(t *testing.T) {
	chdirToTemp(t)
	s := New()
	p := protocol.Player{Name: "bob", Game: "a.zip", InstanceID: "inst-1"}
	s.recordSwapApplied("bob", p)
	s.sendSwap(p, SwapSendOptions{})
}

func TestClearAppliedSwapAllowsResend(t *testing.T) {
	s := New()
	p := protocol.Player{Name: "bob", Game: "a.zip"}
	s.recordSwapApplied("bob", p)
	if s.ShouldSendSwap(p, false) {
		t.Fatal("expected dedupe")
	}
	s.ClearAppliedSwap("bob")
	if !s.ShouldSendSwap(p, false) {
		t.Fatal("expected swap after clear")
	}
}
