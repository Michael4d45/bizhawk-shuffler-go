package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func seedAdminAPITest(t *testing.T, base string) {
	t.Helper()
	gamesBody, _ := json.Marshal(map[string]any{
		"games":      []string{"test.zip"},
		"main_games": []map[string]string{{"file": "test.zip"}},
	})
	res, err := http.Post(base+"/api/games", "application/json", bytes.NewReader(gamesBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/games status %d", res.StatusCode)
	}

	playerBody, _ := json.Marshal(map[string]string{"player": "p1"})
	res, err = http.Post(base+"/api/add_player", "application/json", bytes.NewReader(playerBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/add_player status %d", res.StatusCode)
	}
}

func TestAdminAPIParitySessionControls(t *testing.T) {
	ts := StartTestServer(t)
	seedAdminAPITest(t, ts.URL)
	base := ts.URL

	paths := []string{
		"/api/start",
		"/api/pause",
		"/api/toggle_swaps",
		"/api/toggle_countdown",
		"/api/toggle_prevent_same_game",
		"/api/mode/setup",
	}
	for _, path := range paths {
		res, err := http.Post(base+path, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("POST %s status %d", path, res.StatusCode)
		}
	}

	modeBody, _ := json.Marshal(map[string]string{"mode": "sync"})
	res, err := http.Post(base+"/api/mode", "application/json", bytes.NewReader(modeBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/mode status %d", res.StatusCode)
	}

	res, err = http.Post(base+"/api/do_swap", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/do_swap status %d", res.StatusCode)
	}
}

func TestAdminAPIParityPlayerMessaging(t *testing.T) {
	ts := StartTestServer(t)
	seedAdminAPITest(t, ts.URL)
	base := ts.URL

	swapBody, _ := json.Marshal(map[string]string{"player": "p1", "game": "test.zip"})
	res, err := http.Post(base+"/api/swap_player", "application/json", bytes.NewReader(swapBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/swap_player status %d", res.StatusCode)
	}

	msgBody, _ := json.Marshal(map[string]any{"message": "hi", "duration": 3})
	res, err = http.Post(base+"/api/message_all", "application/json", bytes.NewReader(msgBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/message_all status %d", res.StatusCode)
	}

	completedBody, _ := json.Marshal(map[string]string{"game": "test.zip"})
	res, err = http.Post(base+"/api/players/p1/completed_games", "application/json", bytes.NewReader(completedBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST completed_games status %d", res.StatusCode)
	}

	req, err := http.NewRequest(http.MethodDelete, base+"/api/players/p1/completed_games?game="+strings.ReplaceAll("test.zip", " ", "%20"), nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("DELETE completed_games status %d", res.StatusCode)
	}
}

func TestAdminAPIParityConfigAndFiles(t *testing.T) {
	ts := StartTestServer(t)
	seedAdminAPITest(t, ts.URL)
	base := ts.URL

	checkBody, _ := json.Marshal(map[string]string{"player": "p1"})
	res, err := http.Post(base+"/api/check_player_config", "application/json", bytes.NewReader(checkBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/check_player_config status %d, want 400", res.StatusCode)
	}

	updateBody, _ := json.Marshal(map[string]string{"player": "p1", "config": "{}"})
	res, err = http.Post(base+"/api/update_player_config", "application/json", bytes.NewReader(updateBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusBadRequest && res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("POST /api/update_player_config status %d", res.StatusCode)
	}

	res, err = http.Get(base + "/files/list.json")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /files/list.json status %d", res.StatusCode)
	}

	res, err = http.Get(base + "/api/plugins")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/plugins status %d", res.StatusCode)
	}

	res, err = http.Post(base+"/api/open_roms_folder", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/open_roms_folder status %d", res.StatusCode)
	}
}
