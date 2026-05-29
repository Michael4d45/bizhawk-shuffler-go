package serverhost

import (
	"testing"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestValidateNoDuplicateInstanceAssignments(t *testing.T) {
	st := &protocol.ServerState{
		Players: map[string]protocol.Player{
			"a": {Name: "a", InstanceID: "inst-1"},
			"b": {Name: "b", InstanceID: "inst-2"},
		},
	}
	if err := validateNoDuplicateInstanceAssignments(st); err != nil {
		t.Fatal(err)
	}

	st.Players["b"] = protocol.Player{Name: "b", InstanceID: "inst-1"}
	if err := validateNoDuplicateInstanceAssignments(st); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestSelectNextGameRespectsExcludeAndSeed(t *testing.T) {
	games := []string{"a.zip", "b.zip", "c.zip"}
	first := selectNextGame(games, []string{"a.zip"}, 99)
	second := selectNextGame(games, []string{"a.zip"}, 99)
	if first == "" || first != second {
		t.Fatalf("deterministic pick %q vs %q", first, second)
	}
	if first == "a.zip" {
		t.Fatalf("excluded game was selected: %q", first)
	}
	if selectNextGame(nil, nil, 1) != "" {
		t.Fatal("expected empty for no games")
	}
}

func TestFindAvailableInstanceForPlayerPrefersDifferentGame(t *testing.T) {
	chdirToTemp(t)
	s := New()
	h := &SaveModeHandler{server: s}
	instances := []protocol.GameSwapInstance{
		{ID: "i1", Game: "mario.zip"},
		{ID: "i2", Game: "zelda.zip"},
	}
	player := protocol.Player{Name: "bob", Game: "mario.zip", InstanceID: "i1"}
	assigned := map[int]bool{}
	idx, ok := h.findAvailableInstanceForPlayer(player, instances, assigned, true)
	if !ok || instances[idx].Game != "zelda.zip" {
		t.Fatalf("idx=%d ok=%v game=%q", idx, ok, instances[idx].Game)
	}
}

func TestSyncModeHandleSwapAssignsSharedGame(t *testing.T) {
	chdirToTemp(t)
	s := New()
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Mode = protocol.GameModeSync
		st.Games = []string{"a.zip", "b.zip"}
		st.Players["p1"] = protocol.Player{Name: "p1"}
		st.Players["p2"] = protocol.Player{Name: "p2"}
	})
	h := &SyncModeHandler{server: s}
	if err := h.HandleSwap(); err != nil {
		t.Fatal(err)
	}
	st := s.SnapshotState()
	g1, g2 := st.Players["p1"].Game, st.Players["p2"].Game
	if g1 == "" || g1 != g2 {
		t.Fatalf("expected same game for sync mode, got %q and %q", g1, g2)
	}
}

func TestSaveModeHandleSwapReassignsInstances(t *testing.T) {
	chdirToTemp(t)
	s := New()
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Mode = protocol.GameModeSave
		st.GameSwapInstances = []protocol.GameSwapInstance{
			{ID: "i1", Game: "g1.zip"},
			{ID: "i2", Game: "g2.zip"},
		}
		st.Players["p1"] = protocol.Player{Name: "p1", InstanceID: "i1", Game: "g1.zip"}
		st.Players["p2"] = protocol.Player{Name: "p2", InstanceID: "i2", Game: "g2.zip"}
	})
	h := &SaveModeHandler{server: s}
	if err := h.HandleSwap(); err != nil {
		t.Fatal(err)
	}
	st := s.SnapshotState()
	if err := validateNoDuplicateInstanceAssignments(&st); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, name := range []string{"p1", "p2"} {
		id := st.Players[name].InstanceID
		if id == "" {
			t.Fatalf("%s missing instance", name)
		}
		if ids[id] {
			t.Fatalf("duplicate instance %s", id)
		}
		ids[id] = true
	}
}
