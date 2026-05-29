package integration

import (
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/serverhost"
)

// TestServer is a live HTTP server for integration tests.
type TestServer struct {
	URL     string
	DataDir string
	Host    *serverhost.Server
	http    *http.Server
	ln      net.Listener
}

// StartTestServer listens on 127.0.0.1:0 with an isolated data directory.
func StartTestServer(t *testing.T) *TestServer {
	t.Helper()
	dataDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dataDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	host := serverhost.New()
	host.SetOpenInFileManager(func(string) error { return nil })
	host.SetHost("127.0.0.1")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	host.SetPort(addr.Port)

	mux := http.NewServeMux()
	host.RegisterRoutes(mux)
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()

	ts := &TestServer{
		URL:     "http://" + ln.Addr().String(),
		DataDir: dataDir,
		Host:    host,
		http:    srv,
		ln:      ln,
	}
	t.Cleanup(func() { ts.Stop() })
	return ts
}

// Stop shuts down the HTTP server.
func (ts *TestServer) Stop() {
	if ts.http != nil {
		_ = ts.http.Close()
		ts.http = nil
	}
	if ts.ln != nil {
		_ = ts.ln.Close()
		ts.ln = nil
	}
	time.Sleep(50 * time.Millisecond)
}
