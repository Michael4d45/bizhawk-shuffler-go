package hostsession

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"sync"

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
	server        *serverhost.Server
	httpSrv       *http.Server
	listener      net.Listener
	sessionCtx    context.Context
	sessionCancel context.CancelFunc
	serveWG       sync.WaitGroup
	bindHost      string
	bindPort      int
	adminURL      string
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

	probe, err := net.Listen("tcp", fmt.Sprintf("%s:%d", listenHost, hostPort))
	if err != nil {
		return StartResult{}, fmt.Errorf("listen: %w", err)
	}
	actualPort := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()

	sessionCtx, sessionCancel := context.WithCancel(context.Background())
	s.sessionCtx = sessionCtx
	s.sessionCancel = sessionCancel

	s.server = serverhost.New()
	s.server.SetHost(host)
	s.server.SetPort(actualPort)

	mux := http.NewServeMux()
	s.server.RegisterRoutes(mux)

	addr := fmt.Sprintf("%s:%d", listenHost, actualPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		_ = s.Stop()
		return StartResult{}, fmt.Errorf("listen: %w", err)
	}
	s.listener = ln

	httpSrv := &http.Server{
		Handler: mux,
		BaseContext: func(net.Listener) context.Context {
			return sessionCtx
		},
	}
	s.httpSrv = httpSrv
	s.serveWG.Add(1)
	go func() {
		defer s.serveWG.Done()
		err := httpSrv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("hostsession: serve ended: %v", err)
		}
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

// Stop shuts down the embedded server.
func (s *Session) Stop() error {
	if s == nil {
		return nil
	}

	if s.server != nil {
		s.server.BeginShutdown()
	}

	// Stop accepting new connections first.
	if s.listener != nil {
		_ = s.listener.Close()
		s.listener = nil
	}

	// Cancel in-flight /ws request contexts (admin UI, players).
	if s.sessionCancel != nil {
		log.Printf("hostsession: cancelling session context")
		s.sessionCancel()
		s.sessionCancel = nil
		s.sessionCtx = nil
	}

	if s.server != nil {
		log.Printf("hostsession: draining server")
		s.server.Shutdown()
		log.Printf("hostsession: server drained")
	}

	if s.httpSrv != nil {
		log.Printf("hostsession: closing http server")
		// Close, not Shutdown: upgraded /ws handlers may still be unwinding; Shutdown can block forever.
		if err := s.httpSrv.Close(); err != nil {
			log.Printf("hostsession: http close: %v", err)
		}
		s.httpSrv = nil
	}
	s.serveWG.Wait()

	s.server = nil
	s.bindHost = ""
	s.bindPort = 0
	s.adminURL = ""
	log.Printf("hostsession: stopped")
	return nil
}
