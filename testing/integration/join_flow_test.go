package integration

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/clienthost"
	"github.com/michael4d45/bizshuffle/protocol"
	"github.com/michael4d45/bizshuffle/testing/fakes"
)

func TestReadyWithoutCatalogNoSwap(t *testing.T) {
	ts := StartTestServer(t)
	base := ts.URL

	client := NewWSTestClient(base)
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.Hello("lonely", false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	if err := client.Send(protocol.Command{
		Cmd:     protocol.CmdStatusUpdate,
		ID:      "status-1",
		Payload: map[string]any{"bizhawk_ready": true},
	}); err != nil {
		t.Fatal(err)
	}

	if err := client.WaitNoSwap(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	game, _ := playerAssignment(t, base, "lonely")
	if game != "" {
		t.Fatalf("expected no game assigned, got %q", game)
	}
}

func TestJoinAssignsGameAndSwapOnBizhawkReady(t *testing.T) {
	ts := StartTestServer(t)
	base := ts.URL
	const game = "join-flow.zip"

	seedSyncGames(t, base, []string{game})
	postAddPlayer(t, base, "joiner")
	writeTestROM(t, ts.DataDir, game)

	client := NewWSTestClient(base)
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.Hello("joiner", false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if client.CountInbox(func(c protocol.Command) bool { return c.Cmd == protocol.CmdSwap }) > 0 {
		t.Fatal("swap before bizhawk ready")
	}

	if err := client.Send(protocol.Command{
		Cmd:     protocol.CmdStatusUpdate,
		ID:      "status-2",
		Payload: map[string]any{"bizhawk_ready": true},
	}); err != nil {
		t.Fatal(err)
	}

	cmd, err := client.WaitFor(func(c protocol.Command) bool {
		return c.Cmd == protocol.CmdSwap
	}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := cmd.Payload.(map[string]any)
	if payload["game"] != game {
		t.Fatalf("swap game %+v", payload)
	}
	if got, _ := playerAssignment(t, base, "joiner"); got != game {
		t.Fatalf("persisted game %q", got)
	}
}

func TestSwapReachesFakeLuaAfterBizhawkReady(t *testing.T) {
	ts := StartTestServer(t)
	base := ts.URL
	const game = "lua-swap.zip"
	const player = "lua-joiner"

	seedSyncGames(t, base, []string{game})
	postAddPlayer(t, base, player)
	writeTestROM(t, ts.DataDir, game)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(ts.DataDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	if err := clienthost.EnsureDataDirs(ts.DataDir); err != nil {
		t.Fatal(err)
	}

	bipc, err := clienthost.NewBizhawkIPC(ts.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bipc.Close() })

	peer, err := fakes.StartFakeLuaPeerOnPort(bipc.Port(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	bipc.SetBizhawkLaunched(true)
	if err := bipc.Start(ctx); err != nil {
		t.Fatal(err)
	}

	cfg := clienthost.Config{"name": player, "server": base}
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
		t.Fatal("timeout waiting for ws hello")
	}

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) && !peer.WaitForCommand("SWAP", 200*time.Millisecond) {
		time.Sleep(100 * time.Millisecond)
	}
	if !peer.WaitForCommand("SWAP", 500*time.Millisecond) {
		t.Fatalf("Lua never received SWAP; ipc cmds=%v raw=%v", peer.ReceivedCommands(), peer.Lines())
	}

	ws.Stop()
}
