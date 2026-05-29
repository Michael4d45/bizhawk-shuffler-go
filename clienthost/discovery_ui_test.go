package clienthost

import (
	"testing"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestServerURLsMatch(t *testing.T) {
	if !ServerURLsMatch("http://127.0.0.1:8080", "http://localhost:8080/") {
		t.Fatal("expected match")
	}
	if ServerURLsMatch("http://127.0.0.1:8080", "http://127.0.0.1:9090") {
		t.Fatal("expected different ports")
	}
}

func TestMergeDiscoveredServersHostedInjection(t *testing.T) {
	servers := []*protocol.ServerInfo{
		{Host: "192.168.1.5", Port: 8080, Name: "LAN"},
	}
	out := MergeDiscoveredServers(servers, "http://127.0.0.1:8080", "My Host")
	if len(out) != 2 {
		t.Fatalf("got %d entries", len(out))
	}
	if !out[0].IsHosted || out[0].URL != "http://127.0.0.1:8080" {
		t.Fatalf("first %+v", out[0])
	}
}
