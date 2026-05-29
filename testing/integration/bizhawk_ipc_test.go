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
	_, _ = fmt.Fprintf(conn, "CMD|1|PING\n")
	time.Sleep(50 * time.Millisecond)
	if !peer.WaitForCommand("PING", 2*time.Second) {
		t.Fatalf("got lines %v cmds %v", peer.Lines(), peer.ReceivedCommands())
	}
}
