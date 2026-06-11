package serverhost

import (
	"testing"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestPlayerNameHashIndexStable(t *testing.T) {
	a := playerNameHashIndex("Alice", 7)
	b := playerNameHashIndex("alice", 7)
	if a != b {
		t.Fatalf("hash should be case-insensitive: %d vs %d", a, b)
	}
	if a < 0 || a >= 7 {
		t.Fatalf("index out of range: %d", a)
	}
}

func TestSaveModeGetPlayerUsesNameHash(t *testing.T) {
	chdirToTemp(t)
	s := New()
	instances := []protocol.GameSwapInstance{
		{ID: "i0", Game: "g0.zip"},
		{ID: "i1", Game: "g1.zip"},
		{ID: "i2", Game: "g2.zip"},
	}
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Mode = protocol.GameModeSave
		st.PlayerNameHashAssignment = true
		st.GameSwapInstances = instances
		st.Players["alice"] = protocol.Player{Name: "alice"}
	})
	h := &SaveModeHandler{server: s}
	p := h.GetPlayer("alice")
	wantIdx := playerNameHashIndex("alice", len(instances))
	if p.InstanceID != instances[wantIdx].ID {
		t.Fatalf("instance %q want %q", p.InstanceID, instances[wantIdx].ID)
	}
}

func TestSaveModeNameHashFallsBackWhenPreferredTaken(t *testing.T) {
	chdirToTemp(t)
	s := New()
	instances := []protocol.GameSwapInstance{
		{ID: "only", Game: "g.zip"},
	}
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Mode = protocol.GameModeSave
		st.PlayerNameHashAssignment = true
		st.GameSwapInstances = instances
		st.Players["alice"] = protocol.Player{Name: "alice", InstanceID: "only", Game: "g.zip"}
	})
	h := &SaveModeHandler{server: s}
	p := h.GetPlayer("bob")
	if p.InstanceID != "" {
		t.Fatalf("expected no free instance for bob, got %q", p.InstanceID)
	}
}

func TestSaveModeHandleSwapDeterministicWithNameHash(t *testing.T) {
	chdirToTemp(t)
	run := func() map[string]string {
		s := New()
		s.UpdateStateAndPersist(func(st *protocol.ServerState) {
			st.Mode = protocol.GameModeSave
			st.PlayerNameHashAssignment = true
			st.GameSwapInstances = []protocol.GameSwapInstance{
				{ID: "i0", Game: "g0.zip"},
				{ID: "i1", Game: "g1.zip"},
				{ID: "i2", Game: "g2.zip"},
			}
			st.Players["alice"] = protocol.Player{Name: "alice", InstanceID: "i0", Game: "g0.zip"}
			st.Players["bob"] = protocol.Player{Name: "bob", InstanceID: "i1", Game: "g1.zip"}
		})
		h := &SaveModeHandler{server: s}
		if err := h.HandleSwap(); err != nil {
			t.Fatal(err)
		}
		st := s.SnapshotState()
		out := map[string]string{
			"alice": st.Players["alice"].InstanceID,
			"bob":   st.Players["bob"].InstanceID,
		}
		return out
	}

	first := run()
	second := run()
	if first["alice"] != second["alice"] || first["bob"] != second["bob"] {
		t.Fatalf("non-deterministic assignments: %v vs %v", first, second)
	}
	if first["alice"] == "" || first["bob"] == "" || first["alice"] == first["bob"] {
		t.Fatalf("bad assignments: %v", first)
	}
}
