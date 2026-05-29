package hostsession

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"regexp"

	"github.com/michael4d45/bizshuffle/serverhost"
)

var bindHostPattern = regexp.MustCompile(`^[\da-fA-F:.%-]+$`)

// StartResult is returned after a successful Host.
type StartResult struct {
	AdminURL string
	BindHost string
	HostPort int
}

// Session holds an embedded BizShuffle server for desktop Host.
type Session struct {
	server     *serverhost.Server
	httpSrv    *http.Server
	bindHost   string
	bindPort   int
	adminURL   string
	broadcastC context.CancelFunc
}

// NormalizeBindHost validates and normalizes a bind address.
func NormalizeBindHost(raw string) (string, error) {
	host := raw
	if host == "" {
		host = "127.0.0.1"
	}
	if !bindHostPattern.MatchString(host) {
		return "", fmt.Errorf("invalid bind address: %s", raw)
	}
	return host, nil
}

// LocalAdminURL returns a URL suitable for opening admin in a local browser.
func LocalAdminURL(bindHost string, port int) string {
	switch bindHost {
	case "0.0.0.0", "::", "[::]":
		return fmt.Sprintf("http://127.0.0.1:%d/", port)
	default:
		return fmt.Sprintf("http://%s:%d/", bindHost, port)
	}
}

// HostedURL returns the HTTP URL for this hosted session, if running.
func (s *Session) HostedURL() string {
	if s == nil || s.server == nil {
		return ""
	}
	if s.bindHost == "0.0.0.0" || s.bindHost == "::" || s.bindHost == "[::]" {
		return fmt.Sprintf("http://127.0.0.1:%d", s.bindPort)
	}
	return fmt.Sprintf("http://%s:%d", s.bindHost, s.bindPort)
}

// IsRunning reports whether a host session is active.
func (s *Session) IsRunning() bool {
	return s != nil && s.server != nil && s.httpSrv != nil
}

// Start starts or restarts the embedded server on the requested bind host/port.
// hostPort 0 picks a free TCP port.
func (s *Session) Start(ctx context.Context, bindHost string, hostPort int) (StartResult, error) {
	host, err := NormalizeBindHost(bindHost)
	if err != nil {
		return StartResult{}, err
	}
	if hostPort < 0 || hostPort > 65535 {
		return StartResult{}, fmt.Errorf("invalid port: %d", hostPort)
	}

	if s.IsRunning() && s.bindHost == host && s.bindPort == hostPort {
		return StartResult{AdminURL: s.adminURL, BindHost: host, HostPort: s.bindPort}, nil
	}
	if err := s.Stop(); err != nil {
		return StartResult{}, err
	}

	listenHost := host
	if listenHost == "0.0.0.0" || listenHost == "::" || listenHost == "[::]" {
		listenHost = "0.0.0.0"
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", listenHost, hostPort))
	if err != nil {
		return StartResult{}, fmt.Errorf("listen: %w", err)
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	s.server = serverhost.New()
	s.server.SetHost(host)
	s.server.SetPort(actualPort)

	mux := http.NewServeMux()
	s.server.RegisterRoutes(mux)

	bcastCtx, cancel := context.WithCancel(ctx)
	s.broadcastC = cancel
	if err := s.server.StartBroadcaster(bcastCtx); err != nil {
		cancel()
		s.server = nil
		return StartResult{}, err
	}

	addr := fmt.Sprintf("%s:%d", listenHost, actualPort)
	s.httpSrv = &http.Server{Addr: addr, Handler: mux}
	go func() {
		ln2, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		_ = s.httpSrv.Serve(ln2)
	}()

	if _, err := s.server.SyncCatalogFromRoms(); err != nil {
		_ = s.Stop()
		return StartResult{}, err
	}

	s.bindHost = host
	s.bindPort = actualPort
	s.adminURL = LocalAdminURL(host, actualPort)

	return StartResult{
		AdminURL: s.adminURL,
		BindHost: host,
		HostPort: actualPort,
	}, nil
}

// Stop shuts down the embedded server and discovery broadcaster.
func (s *Session) Stop() error {
	if s == nil {
		return nil
	}
	if s.broadcastC != nil {
		s.broadcastC()
		s.broadcastC = nil
	}
	if s.server != nil {
		_ = s.server.StopBroadcaster()
	}
	var err error
	if s.httpSrv != nil {
		err = s.httpSrv.Close()
		s.httpSrv = nil
	}
	s.server = nil
	s.bindHost = ""
	s.bindPort = 0
	s.adminURL = ""
	return err
}
