package updates

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	if CompareVersions("1.0.0", "1.0.1") >= 0 {
		t.Fatal("expected less")
	}
	if CompareVersions("2.0", "1.9.9") <= 0 {
		t.Fatal("expected greater")
	}
	if CompareVersions("1.0.0", "1.0.0") != 0 {
		t.Fatal("expected equal")
	}
}

func TestCheckLatestUpdateAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ghRelease{
			TagName: "v2.0.0",
			HTMLURL: "https://github.com/example/releases/tag/v2.0.0",
			Assets: []struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
			}{{Name: "bizshuffle-desktop.exe", URL: "https://example.com/desktop.exe"}},
		})
	}))
	defer srv.Close()

	st, err := checkReleaseJSON(context.Background(), srv.URL, "1.0.0", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if !st.UpdateAvailable || st.LatestVersion != "2.0.0" {
		t.Fatalf("got %+v", st)
	}
	if st.DownloadURL != "https://example.com/desktop.exe" {
		t.Fatalf("download %q", st.DownloadURL)
	}
}

func TestCheckLatestNoUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ghRelease{TagName: "v1.0.0", HTMLURL: "https://example.com"})
	}))
	defer srv.Close()
	st, err := checkReleaseJSON(context.Background(), srv.URL, "1.0.0", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if st.UpdateAvailable {
		t.Fatalf("got %+v", st)
	}
}

func checkReleaseJSON(ctx context.Context, url, current string, client *http.Client) (State, error) {
	st := State{Version: formatVersion(current)}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return st, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return st, err
	}
	defer func() { _ = resp.Body.Close() }()
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return st, err
	}
	latest := strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	cur := formatVersion(current)
	st.LatestVersion = latest
	if latest != "" && CompareVersions(cur, latest) < 0 {
		st.UpdateAvailable = true
		st.DownloadURL = rel.HTMLURL
		for _, a := range rel.Assets {
			if strings.Contains(strings.ToLower(a.Name), "bizshuffle-desktop") {
				st.DownloadURL = a.URL
				break
			}
		}
	}
	return st, nil
}
