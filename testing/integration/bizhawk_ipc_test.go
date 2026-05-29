package integration_test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/testing/fakes"
)

func TestFakeLuaPeerAcceptsLines(t *testing.T) {
	peer, port, err := fakes.StartFakeLuaPeer()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = fmt.Fprintf(conn, "PING\n")
	time.Sleep(50 * time.Millisecond)
	lines := peer.Lines()
	if len(lines) != 1 || lines[0] != "PING" {
		t.Fatalf("got %v", lines)
	}
}
