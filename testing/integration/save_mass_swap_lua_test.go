package integration

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/clienthost"
	"github.com/michael4d45/bizshuffle/protocol"
	"github.com/michael4d45/bizshuffle/testing/fakes"
)

// TS parity: packages/testing/src/integration/save-swap-two-player.test.ts uses real
// ClientRuntime + FakeLuaPeer and would fail if SAVE is spammed during performSwap.
func TestSaveModeMassSwapSendsAtMostOneSavePerPlayerToLua(t *testing.T) {
	ts := StartTestServer(t)
	base := ts.URL

	writeTestROM(t, ts.DataDir, "Banjo-Kazooie (USA).zip")
	writeTestROM(t, ts.DataDir, "Chrono Trigger (USA).zip")

	res, err := postJSON(base, "/api/mode", map[string]string{"mode": "save"})
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	postGameInstances(t, base, []map[string]any{
		{"id": banjoInstanceID, "game": "Banjo-Kazooie (USA).zip", "file_state": "none"},
		{"id": chronoInstanceID, "game": "Chrono Trigger (USA).zip", "file_state": "none"},
	})
	postAddPlayer(t, base, "bob")
	postAddPlayer(t, base, "test")

	bob := NewWSTestClient(base)
	bob.SaveUploadBase = base
	if err := bob.Connect(); err != nil {
		t.Fatal(err)
	}
	defer bob.Close()
	if err := bob.Hello("bob", true); err != nil {
		t.Fatal(err)
	}

	clientDir := filepath.Join(ts.DataDir, "client-test")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldWd, _ := os.Getwd()
	if err := os.Chdir(clientDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	if err := clienthost.EnsureDataDirs(clientDir); err != nil {
		t.Fatal(err)
	}

	bipc, err := clienthost.NewBizhawkIPC(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bipc.Close() })

	peer, err := fakes.StartFakeLuaPeerOnPort(bipc.Port(), &fakes.FakeLuaSaveOpts{
		SavesDir:   clientDir,
		InstanceID: chronoInstanceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)

	bipc.SetBizhawkLaunched(true)
	if err := bipc.Start(ctx); err != nil {
		t.Fatal(err)
	}

	cfg := clienthost.Config{"name": "test", "server": base}
	api := clienthost.NewAPI(base, &http.Client{Timeout: 30 * time.Second}, cfg)
	ws := clienthost.NewWSClient(HTTPToWS(base), api, bipc)
	bh := clienthost.NewBizHawkController(api, &http.Client{Timeout: 30 * time.Second}, cfg, bipc, ws)
	bh.SetOnBizhawkReady(func() {
		if ctrl := ws.GetController(); ctrl != nil {
			ctrl.OnBizhawkReady(ctx)
		}
	})
	bh.StartIPCGoroutine(ctx)

	done := make(chan struct{})
	go func() {
		ws.Start(ctx, cfg)
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timeout waiting for test player ws hello")
	}

	ts.Host.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.PreventSameGameSwap = true
		for _, a := range []struct{ name, inst, game string }{
			{"bob", banjoInstanceID, "Banjo-Kazooie (USA).zip"},
			{"test", chronoInstanceID, "Chrono Trigger (USA).zip"},
		} {
			p := st.Players[a.name]
			p.Game = a.game
			p.InstanceID = a.inst
			p.Connected = true
			p.BizhawkReady = true
			st.Players[a.name] = p
		}
	})

	settle := time.Now().Add(15 * time.Second)
	for time.Now().Before(settle) && ts.Host.PendingCommandCount() > 0 {
		time.Sleep(50 * time.Millisecond)
	}

	saveBefore := peer.CountCommand("SAVE")

	res, err = http.Post(base+"/api/do_swap", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	swapDeadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(swapDeadline) {
		st := ts.Host.SnapshotState()
		if st.Players["bob"].InstanceID == chronoInstanceID && st.Players["test"].InstanceID == banjoInstanceID {
			break
		}
		if ts.Host.PendingCommandCount() == 0 && ts.Host.PendingInstanceCount() == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	waitSave := time.Now().Add(10 * time.Second)
	for time.Now().Before(waitSave) && peer.CountCommand("SAVE") <= saveBefore {
		time.Sleep(50 * time.Millisecond)
	}

	saveDuringMassSwap := peer.CountCommand("SAVE") - saveBefore
	if saveDuringMassSwap == 0 {
		t.Fatal("test player Lua peer never received SAVE during mass swap")
	}
	if saveDuringMassSwap > 1 {
		t.Fatalf("SAVE spam: Lua received %d SAVE during mass swap (want 1); cmds=%v",
			saveDuringMassSwap, peer.ReceivedCommands())
	}

	if n := bob.CountInbox(func(c protocol.Command) bool { return c.Cmd == protocol.CmdRequestSave }); n > 1 {
		t.Fatalf("bob received %d request_save WS messages during mass swap (want at most 1)", n)
	}

	ws.Stop()
}
