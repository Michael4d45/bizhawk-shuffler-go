package integration

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
)

const (
	banjoInstanceID = "banjo-kazooie--usa-"
	chronoInstanceID = "chrono-trigger--usa-"
)

func TestSaveModeTwoPlayerSwap(t *testing.T) {
	ts := StartTestServer(t)
	base := ts.URL

	res, err := postJSON(base, "/api/mode", map[string]string{"mode": "save"})
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/mode status %d", res.StatusCode)
	}

	postGameInstances(t, base, []map[string]any{
		{"id": banjoInstanceID, "game": "Banjo-Kazooie (USA).zip", "file_state": "none"},
		{"id": chronoInstanceID, "game": "Chrono Trigger (USA).zip", "file_state": "none"},
	})

	for _, player := range []string{"bob", "test"} {
		res, err := postJSON(base, "/api/add_player", map[string]string{"player": player})
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("POST /api/add_player %s status %d", player, res.StatusCode)
		}
	}

	bob := NewWSTestClient(base)
	bob.SaveUploadBase = base
	testClient := NewWSTestClient(base)
	testClient.SaveUploadBase = base

	for _, c := range []*WSTestClient{bob, testClient} {
		if err := c.Connect(); err != nil {
			t.Fatal(err)
		}
		defer c.Close()
	}

	if err := bob.Hello("bob", true); err != nil {
		t.Fatal(err)
	}
	if err := testClient.Hello("test", true); err != nil {
		t.Fatal(err)
	}

	ts.Host.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.PreventSameGameSwap = true
		assign := []struct {
			name, instanceID, game string
		}{
			{"bob", banjoInstanceID, "Banjo-Kazooie (USA).zip"},
			{"test", chronoInstanceID, "Chrono Trigger (USA).zip"},
		}
		for _, a := range assign {
			p := st.Players[a.name]
			p.Game = a.game
			p.InstanceID = a.instanceID
			p.Connected = true
			p.BizhawkReady = true
			st.Players[a.name] = p
		}
	})

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && ts.Host.PendingCommandCount() > 0 {
		time.Sleep(50 * time.Millisecond)
	}
	if ts.Host.PendingCommandCount() > 0 {
		t.Fatalf("connect swaps still pending: %d", ts.Host.PendingCommandCount())
	}

	res, err = http.Post(base+"/api/do_swap", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/do_swap status %d", res.StatusCode)
	}

	swapDeadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(swapDeadline) {
		st := ts.Host.SnapshotState()
		bobP := st.Players["bob"]
		testP := st.Players["test"]
		if bobP.InstanceID == chronoInstanceID && testP.InstanceID == banjoInstanceID {
			break
		}
		if ts.Host.PendingCommandCount() == 0 &&
			ts.Host.PendingInstanceCount() == 0 &&
			bobP.InstanceID != "" && testP.InstanceID != "" &&
			bobP.InstanceID != banjoInstanceID {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	st := ts.Host.SnapshotState()
	if st.Players["bob"].InstanceID != chronoInstanceID {
		t.Fatalf("bob instance %q want %q", st.Players["bob"].InstanceID, chronoInstanceID)
	}
	if st.Players["test"].InstanceID != banjoInstanceID {
		t.Fatalf("test instance %q want %q", st.Players["test"].InstanceID, banjoInstanceID)
	}

	fileStates := map[string]string{}
	for _, inst := range st.GameSwapInstances {
		fileStates[inst.ID] = string(inst.FileState)
	}
	if fileStates[banjoInstanceID] != string(protocol.FileStateReady) {
		t.Fatalf("banjo file_state %q", fileStates[banjoInstanceID])
	}
	if fileStates[chronoInstanceID] != string(protocol.FileStateReady) {
		t.Fatalf("chrono file_state %q", fileStates[chronoInstanceID])
	}

	for _, id := range []string{banjoInstanceID, chronoInstanceID} {
		path := filepath.Join(ts.DataDir, "saves", id+".state")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing save %s: %v", path, err)
		}
	}

	for name, c := range map[string]*WSTestClient{"bob": bob, "test": testClient} {
		n := c.CountInbox(func(cmd protocol.Command) bool {
			return cmd.Cmd == protocol.CmdRequestSave
		})
		if n == 0 {
			t.Fatalf("%s never received request_save during mass swap", name)
		}
		if n > 1 {
			t.Fatalf("%s received %d request_save during mass swap (TS sends once before wait, not per poll tick)", name, n)
		}
	}
}
