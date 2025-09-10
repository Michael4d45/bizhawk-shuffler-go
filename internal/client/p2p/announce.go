package p2p

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	pt "github.com/michael4d45/bizshuffle/internal/types"
)

// AnnounceClient periodically announces locally present save states to the server.
// It relies on the existing HTTP API base URL. This does NOT handle peer downloads; it
// only informs the server tracker which instances this client seeds.
type AnnounceClient struct {
	baseURL    string
	httpClient *http.Client
	interval   time.Duration
	peerID     string
	cancel     context.CancelFunc
	running    bool
}

// NewAnnounceClient creates an announce loop with the given interval (minimum 5s enforced).
func NewAnnounceClient(baseURL, peerID string, httpClient *http.Client, interval time.Duration) *AnnounceClient {
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	return &AnnounceClient{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient, interval: interval, peerID: peerID}
}

// Start begins background announce loop. Safe to call once.
func (a *AnnounceClient) Start(ctx context.Context) {
	if a.running {
		return
	}
	ctx, a.cancel = context.WithCancel(ctx)
	a.running = true
	go a.loop(ctx)
	log.Printf("[P2P][announce_loop] started interval=%s", a.interval)
}

// Stop terminates background loop.
func (a *AnnounceClient) Stop() {
	if !a.running {
		return
	}
	a.running = false
	if a.cancel != nil {
		a.cancel()
	}
	log.Printf("[P2P][announce_loop] stopped")
}

func (a *AnnounceClient) loop(ctx context.Context) {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	for {
		if err := a.doAnnounce(ctx); err != nil {
			// Distinguish expected transient network errors
			log.Printf("[P2P][announce_loop][unexpected] announce error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// doAnnounce performs a single announce HTTP POST.
func (a *AnnounceClient) doAnnounce(ctx context.Context) error {
	if a.peerID == "" {
		return fmt.Errorf("empty peerID")
	}
	instances, err := a.collectLocalInstances()
	if err != nil {
		return err
	}
	payload := struct {
		PeerID    string                `json:"peer_id"`
		Instances []pt.SaveStateVersion `json:"instances"`
	}{PeerID: a.peerID, Instances: instances}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/api/p2p/save-announce", strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	log.Printf("[P2P][announce_loop] announced peer=%s instances=%d duration=%s", a.peerID, len(instances), time.Since(start).Truncate(time.Millisecond))
	return nil
}

// collectLocalInstances scans ./saves and returns SaveStateVersion slices for present files.
func (a *AnnounceClient) collectLocalInstances() ([]pt.SaveStateVersion, error) {
	entries, err := os.ReadDir("./saves")
	if err != nil {
		return nil, err
	}
	out := make([]pt.SaveStateVersion, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".state") {
			continue
		}
		id := strings.TrimSuffix(name, ".state")
		path := filepath.Join("./saves", name)
		st, err := os.Stat(path)
		if err != nil {
			log.Printf("[P2P][announce_loop][expected] stat failed path=%s err=%v", path, err)
			continue
		}
		hash, err := hashFile(path)
		if err != nil {
			log.Printf("[P2P][announce_loop][unexpected] hash error path=%s err=%v", path, err)
			continue
		}
		out = append(out, pt.SaveStateVersion{InstanceID: id, Hash: hash, Size: st.Size(), UpdatedAt: st.ModTime()})
	}
	return out, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
