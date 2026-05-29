package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"testing"
)

func TestAdminStaticAssets(t *testing.T) {
	ts := StartTestServer(t)

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET / status %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`src="(/assets/[^"]+)"`)
	m := re.FindStringSubmatch(string(body))
	if len(m) < 2 {
		t.Fatal("index.html missing /assets/ script src")
	}
	assetRes, err := http.Get(ts.URL + m[1])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = assetRes.Body.Close() }()
	if assetRes.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status %d", m[1], assetRes.StatusCode)
	}
}

func TestStateJSON(t *testing.T) {
	ts := StartTestServer(t)

	res, err := http.Get(ts.URL + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var out struct {
		State struct {
			Running bool `json:"running"`
		} `json:"state"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.State.Running {
		t.Fatal("expected running false on fresh server")
	}
}

func TestGETShareURLs(t *testing.T) {
	ts := StartTestServer(t)

	res, err := http.Get(ts.URL + "/api/share_urls")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var body struct {
		LAN       []string `json:"lan"`
		WAN       *string  `json:"wan"`
		LocalOnly bool     `json:"local_only"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.LocalOnly {
		t.Fatalf("expected local_only for 127.0.0.1 bind, got %+v", body)
	}
	if len(body.LAN) != 0 {
		t.Fatalf("expected empty lan, got %+v", body.LAN)
	}
	if body.WAN != nil {
		t.Fatalf("expected nil wan, got %v", *body.WAN)
	}
}
