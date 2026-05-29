package fakes

import (
	"bufio"
	"fmt"
	"net"
	"sync"
)

// FakeLuaPeer is a minimal TCP server that speaks line-based BizHawk IPC for tests.
type FakeLuaPeer struct {
	ln     net.Listener
	mu     sync.Mutex
	lines  []string
	closed bool
}

func StartFakeLuaPeer() (*FakeLuaPeer, int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, err
	}
	p := &FakeLuaPeer{ln: ln}
	go p.serve()
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)
	return p, port, nil
}

func (p *FakeLuaPeer) serve() {
	for {
		conn, err := p.ln.Accept()
		if err != nil {
			return
		}
		go p.handle(conn)
	}
}

func (p *FakeLuaPeer) handle(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		line := sc.Text()
		p.mu.Lock()
		p.lines = append(p.lines, line)
		p.mu.Unlock()
		if _, err := fmt.Fprintf(conn, "ACK\n"); err != nil {
			return
		}
	}
}

func (p *FakeLuaPeer) Lines() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.lines...)
}

func (p *FakeLuaPeer) Close() error {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	return p.ln.Close()
}
