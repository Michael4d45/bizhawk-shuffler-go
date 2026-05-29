package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/michael4d45/bizshuffle/savestate"
)

func TestSaveUploadValidSetsReady(t *testing.T) {
	ts := StartTestServer(t)
	instanceID := "mario-1"
	postGameInstances(t, ts.URL, []map[string]any{
		{"id": instanceID, "game": "mario.zip", "file_state": "none"},
	})

	saveBytes, err := savestate.BuildMinimalBizHawkSavestate()
	if err != nil {
		t.Fatal(err)
	}
	res := uploadSaveMultipart(t, ts.URL, instanceID+".state", saveBytes)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("upload status %d: %s", res.StatusCode, b)
	}

	stRes, err := http.Get(ts.URL + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stRes.Body.Close() }()
	var env struct {
		State struct {
			GameInstances []struct {
				ID        string `json:"id"`
				FileState string `json:"file_state"`
			} `json:"game_instances"`
		} `json:"state"`
	}
	if err := json.NewDecoder(stRes.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	for _, inst := range env.State.GameInstances {
		if inst.ID == instanceID && inst.FileState != "ready" {
			t.Fatalf("file_state %q want ready", inst.FileState)
		}
	}

	dl, err := http.Get(ts.URL + "/save/" + instanceID + ".state")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dl.Body.Close() }()
	if dl.StatusCode != http.StatusOK {
		t.Fatalf("GET save status %d", dl.StatusCode)
	}
	got, _ := io.ReadAll(dl.Body)
	if !bytes.Equal(got, saveBytes) {
		t.Fatal("downloaded bytes differ from upload")
	}
	if _, err := os.Stat(filepath.Join(ts.DataDir, "saves", instanceID+".state")); err != nil {
		t.Fatal(err)
	}
}

func TestSaveUploadRejectsInvalidSavestate(t *testing.T) {
	ts := StartTestServer(t)
	instanceID := "bad-save-1"
	postGameInstances(t, ts.URL, []map[string]any{
		{"id": instanceID, "game": "x.zip", "file_state": "pending"},
	})

	badZip, err := savestate.BuildNonBizHawkZip()
	if err != nil {
		t.Fatal(err)
	}
	res := uploadSaveMultipart(t, ts.URL, instanceID+".state", badZip)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("upload status %d want 422: %s", res.StatusCode, b)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != string(savestate.CodeMissingBizStateVersion) {
		t.Fatalf("code %q", body.Code)
	}

	stRes, err := http.Get(ts.URL + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stRes.Body.Close() }()
	var env struct {
		State struct {
			GameInstances []struct {
				ID        string `json:"id"`
				FileState string `json:"file_state"`
			} `json:"game_instances"`
		} `json:"state"`
	}
	if err := json.NewDecoder(stRes.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	for _, inst := range env.State.GameInstances {
		if inst.ID == instanceID && inst.FileState != "pending" {
			t.Fatalf("file_state %q want pending unchanged", inst.FileState)
		}
	}
}
