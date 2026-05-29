package clienthost

import "testing"

func TestConfigNormalizeServer(t *testing.T) {
	c := Config{"server": "ws://127.0.0.1:8080/ws"}
	c.normalizeServer()
	if c["server"] != "http://127.0.0.1:8080" {
		t.Fatalf("got %q", c["server"])
	}
}
