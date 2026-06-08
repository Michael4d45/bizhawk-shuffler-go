package serverhost

import (
	"sync"
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestWaitForSwapGateReturnsImmediatelyWhenIdle(t *testing.T) {
	s := New()
	h := &SaveModeHandler{server: s}
	if h.waitForSwapGate(500 * time.Millisecond) {
		t.Fatal("expected no pending work")
	}
}

func TestConcurrentRandomSwapsKeepDistinctInstanceAssignments(t *testing.T) {
	chdirToTemp(t)
	s := New()
	startFakePlayerClient(s, "alice")
	startFakePlayerClient(s, "bob")

	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Mode = protocol.GameModeSave
		st.PreventSameGameSwap = true
		st.GameSwapInstances = []protocol.GameSwapInstance{
			{ID: "oot-a", Game: "Ocarina of Time.zip", FileState: protocol.FileStateReady},
			{ID: "oot-b", Game: "Ocarina of Time.zip", FileState: protocol.FileStateReady},
		}
		st.Players = map[string]protocol.Player{
			"alice": {
				Name: "alice", Game: "Ocarina of Time.zip", InstanceID: "oot-a",
				Connected: true, BizhawkReady: true,
			},
			"bob": {
				Name: "bob", Game: "Ocarina of Time.zip", InstanceID: "oot-b",
				Connected: true, BizhawkReady: true,
			},
		}
	})

	h := &SaveModeHandler{server: s}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_ = h.HandleRandomSwapForPlayer("alice")
	}()
	go func() {
		defer wg.Done()
		<-start
		_ = h.HandleRandomSwapForPlayer("bob")
	}()
	close(start)
	wg.Wait()

	st := s.SnapshotState()
	if err := validateNoDuplicateInstanceAssignments(&st); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, name := range []string{"alice", "bob"} {
		id := st.Players[name].InstanceID
		if id == "" {
			t.Fatalf("%s missing instance after concurrent swap_me", name)
		}
		if other, dup := ids[id]; dup {
			t.Fatalf("instance %s assigned to both %s and %s", id, other, name)
		}
		ids[id] = name
	}
	if len(ids) != 2 {
		t.Fatalf("expected two distinct instances, got %v", ids)
	}
}
