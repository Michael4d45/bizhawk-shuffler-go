package serverhost

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestGETShareURLsUsesBindHost(t *testing.T) {
	dataDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dataDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWd)
	})

	s := New()
	s.SetHost("0.0.0.0")
	s.SetPort(8080)

	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	srv := httptest.NewUnstartedServer(mux)
	srv.Listener.Close()
	srv.Config = &http.Server{Handler: mux}
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/share_urls")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var body ShareUrls
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.LocalOnly {
		t.Fatal("expected local_only false for 0.0.0.0 bind")
	}
	if len(body.LAN) == 0 {
		t.Fatal("expected at least one LAN url")
	}
}
