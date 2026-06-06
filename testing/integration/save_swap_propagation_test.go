package integration

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/clienthost"
	"github.com/michael4d45/bizshuffle/protocol"
	"github.com/michael4d45/bizshuffle/savestate"
	"github.com/michael4d45/bizshuffle/testing/fakes"
)

// Mass swap must download the other player's freshly uploaded save instead of reusing
// a stale local .state file for the incoming instance.
func TestSaveModeMassSwapDownloadsIncomingSave(t *testing.T) {
	ts := StartTestServer(t)
	base := ts.URL

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

	freshSave, err := savestate.BuildMinimalBizHawkSavestate()
	if err != nil {
		t.Fatal(err)
	}
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
	// Server shares this process cwd; ROMs must exist after chdir to clientDir.
	writeTestROM(t, clientDir, "Banjo-Kazooie (USA).zip")
	writeTestROM(t, clientDir, "Chrono Trigger (USA).zip")

	staleSave, err := buildTaggedSave(0x01)
	if err != nil {
		t.Fatal(err)
	}
	stalePath := filepath.Join(clientDir, "saves", banjoInstanceID+".state")
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, staleSave, 0o644); err != nil {
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

	res, err = http.Post(base+"/api/do_swap", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

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
			testP.InstanceID != "" &&
			testP.InstanceID != chronoInstanceID {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	st := ts.Host.SnapshotState()
	if st.Players["test"].InstanceID != banjoInstanceID {
		t.Fatalf("test player instance %q want %q", st.Players["test"].InstanceID, banjoInstanceID)
	}

	var got []byte
	waitFile := time.Now().Add(15 * time.Second)
	for time.Now().Before(waitFile) {
		got, err = os.ReadFile(stalePath)
		if err == nil && bytes.Equal(got, freshSave) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("incoming save not downloaded to client: %v", err)
	}
	if bytes.Equal(got, staleSave) {
		t.Fatal("client still has stale local save; expected freshly uploaded save from server")
	}
	if !bytes.Equal(got, freshSave) {
		t.Fatal("downloaded save does not match the save bob uploaded before swap")
	}

	serverSave, err := os.ReadFile(filepath.Join(clientDir, "saves", banjoInstanceID+".state"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, serverSave) {
		t.Fatal("client save does not match server copy")
	}

	ws.Stop()
}
