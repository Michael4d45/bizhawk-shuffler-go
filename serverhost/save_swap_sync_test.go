package serverhost

import (
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestRestorePlayersRevertsAssignment(t *testing.T) {
	chdirToTemp(t)
	s := New()
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Players["alice"] = protocol.Player{Name: "alice", InstanceID: "oot-a"}
	})
	snap := s.snapshotAllPlayers()
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		p := st.Players["alice"]
		p.InstanceID = "oot-b"
		st.Players["alice"] = p
	})
	s.restorePlayers(snap)
	if got := s.SnapshotState().Players["alice"].InstanceID; got != "oot-a" {
		t.Fatalf("instance %q want oot-a", got)
	}
}

func TestSendSwapSyncWaitsForInFlight(t *testing.T) {
	chdirToTemp(t)
	s := New()
	startFakePlayerClient(s, "bob")
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Players["bob"] = protocol.Player{
			Name: "bob", Game: "a.zip", InstanceID: "inst-1",
			Connected: true, BizhawkReady: true,
		}
	})
	s.withLock(func() { s.swapInFlight["bob"] = struct{}{} })

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.withLock(func() { delete(s.swapInFlight, "bob") })
	}()
	time.Sleep(50 * time.Millisecond)

	err := s.sendSwapSync(s.currentPlayer("bob"), SwapSendOptions{Force: true})
	<-done
	if err != nil {
		t.Fatal(err)
	}
}
