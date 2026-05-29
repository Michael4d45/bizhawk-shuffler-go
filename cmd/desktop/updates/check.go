package updates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// State is the desktop app update UI state.
type State struct {
	Version         string
	LatestVersion   string
	UpdateAvailable bool
	DownloadURL     string
	Error           string
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// DefaultRepo is the GitHub repository for release checks.
const DefaultRepo = "michael4d45/bizshuffle"

// CheckLatest compares the embedded version with the latest GitHub release tag.
func CheckLatest(ctx context.Context, repo, current string, client *http.Client) (State, error) {
	st := State{Version: formatVersion(current)}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if repo == "" {
		repo = DefaultRepo
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return st, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "bizshuffle-desktop")

	resp, err := client.Do(req)
	if err != nil {
		st.Error = err.Error()
		return st, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		st.Error = fmt.Sprintf("github api %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		return st, fmt.Errorf("%s", st.Error)
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return st, err
	}
	latest := strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	cur := strings.TrimPrefix(formatVersion(current), "v")
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

func formatVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "0.0.0"
	}
	return strings.TrimPrefix(v, "v")
}

// VersionLabel returns a short label for the footer.
func VersionLabel(st State) string {
	ver := formatVersion(st.Version)
	if st.UpdateAvailable && st.LatestVersion != "" && st.LatestVersion != ver {
		return fmt.Sprintf("v%s → v%s", ver, st.LatestVersion)
	}
	if ver == "dev" || strings.HasSuffix(ver, "dev") {
		return "v" + ver + " (dev)"
	}
	return "v" + ver
}

// CompareVersions returns -1 if a < b, 0 if equal, 1 if a > b (numeric dot segments).
func CompareVersions(a, b string) int {
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	for len(ap) < len(bp) {
		ap = append(ap, "0")
	}
	for len(bp) < len(ap) {
		bp = append(bp, "0")
	}
	for i := range ap {
		ai, _ := strconv.Atoi(ap[i])
		bi, _ := strconv.Atoi(bp[i])
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}
