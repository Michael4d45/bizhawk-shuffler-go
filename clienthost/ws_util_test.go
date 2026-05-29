package clienthost

import "testing"

func TestBuildWSAndHTTPFromHTTP(t *testing.T) {
	cfg := Config{}
	ws, httpBase, err := BuildWSAndHTTP("http://127.0.0.1:9000", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if httpBase != "http://127.0.0.1:9000" {
		t.Fatalf("http %q", httpBase)
	}
	if ws != "ws://127.0.0.1:9000/ws" {
		t.Fatalf("ws %q", ws)
	}
}

func TestBuildWSAndHTTPFromWSFlag(t *testing.T) {
	cfg := Config{}
	ws, httpBase, err := BuildWSAndHTTP("ws://127.0.0.1:9000/ws", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if ws != "ws://127.0.0.1:9000/ws" {
		t.Fatalf("ws %q", ws)
	}
	if httpBase != "http://127.0.0.1:9000" {
		t.Fatalf("http %q", httpBase)
	}
}
