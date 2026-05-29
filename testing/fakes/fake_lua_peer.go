package fakes

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/michael4d45/bizshuffle/savestate"
)

// FakeLuaSaveOpts enables SAVE handling that writes a minimal .state file (TS FakeLuaPeer parity).
type FakeLuaSaveOpts struct {
	SavesDir   string
	InstanceID string
}

// FakeLuaPeer is a minimal TCP server that speaks line-based BizHawk IPC for tests.
type FakeLuaPeer struct {
	ln         net.Listener
	mu         sync.Mutex
	lines      []string
	closed     bool
	savesDir   string
	instanceID string
}

// StartFakeLuaPeer listens on 127.0.0.1:0 and returns the bound port.
func StartFakeLuaPeer() (*FakeLuaPeer, int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, err
	}
	return startPeerOnListener(ln, nil)
}

// StartFakeLuaPeerOnPort listens on 127.0.0.1:port (for BizhawkIPC port file parity).
func StartFakeLuaPeerOnPort(port int, saveOpts *FakeLuaSaveOpts) (*FakeLuaPeer, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, err
	}
	p, _, err := startPeerOnListener(ln, saveOpts)
	return p, err
}

func startPeerOnListener(ln net.Listener, saveOpts *FakeLuaSaveOpts) (*FakeLuaPeer, int, error) {
	p := &FakeLuaPeer{ln: ln}
	if saveOpts != nil {
		p.savesDir = saveOpts.SavesDir
		p.instanceID = saveOpts.InstanceID
	}
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
	_, _ = fmt.Fprintf(conn, "HELLO\n")
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		p.mu.Lock()
		p.lines = append(p.lines, line)
		p.mu.Unlock()
		if strings.HasPrefix(line, "CMD|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				id := parts[1]
				cmd := parts[2]
				p.mu.Lock()
				p.lines = append(p.lines, "CMD:"+cmd)
				p.mu.Unlock()
				if cmd == "SWAP" && len(parts) >= 5 {
					p.mu.Lock()
					if parts[4] != "" {
						p.instanceID = parts[4]
					} else if parts[3] != "" {
						p.instanceID = parts[3]
					}
					p.mu.Unlock()
				}
				if cmd == "SAVE" && p.savesDir != "" {
					inst := p.instanceID
					if len(parts) >= 5 && parts[4] != "" {
						inst = parts[4]
					}
					p.mu.Lock()
					p.instanceID = inst
					p.mu.Unlock()
					_ = p.writeMinimalSave(inst)
				}
				_, _ = fmt.Fprintf(conn, "ACK|%s\n", id)
				continue
			}
		}
		_, _ = fmt.Fprintf(conn, "ACK\n")
	}
}

func (p *FakeLuaPeer) writeMinimalSave(instanceID string) error {
	data, err := savestate.BuildMinimalBizHawkSavestate()
	if err != nil {
		return err
	}
	dir := filepath.Join(p.savesDir, "saves")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, instanceID+".state"), data, 0o644)
}

// Lines returns all raw lines received from the controller.
func (p *FakeLuaPeer) Lines() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.lines...)
}

// ReceivedCommands returns CMD verb names (e.g. SWAP, SAVE) in order.
func (p *FakeLuaPeer) ReceivedCommands() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, line := range p.lines {
		if strings.HasPrefix(line, "CMD:") {
			out = append(out, strings.TrimPrefix(line, "CMD:"))
		}
	}
	return out
}

// CountCommand returns how many times a CMD verb was received.
func (p *FakeLuaPeer) CountCommand(cmd string) int {
	n := 0
	for _, c := range p.ReceivedCommands() {
		if c == cmd {
			n++
		}
	}
	return n
}

// WaitForCommand polls until a CMD verb appears or timeout.
func (p *FakeLuaPeer) WaitForCommand(cmd string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, c := range p.ReceivedCommands() {
			if c == cmd {
				return true
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

func (p *FakeLuaPeer) Close() error {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	return p.ln.Close()
}
