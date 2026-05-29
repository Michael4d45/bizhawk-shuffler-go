package obslog

import "testing"

func TestURLsSameHostPort(t *testing.T) {
	if !URLsSameHostPort("http://127.0.0.1:8080", "http://localhost:8080/") {
		t.Fatal("expected same port")
	}
	if URLsSameHostPort("http://127.0.0.1:8080", "http://127.0.0.1:65063") {
		t.Fatal("expected different ports")
	}
}

func TestURLPort(t *testing.T) {
	if got := URLPort("http://127.0.0.1:65063"); got != "65063" {
		t.Fatalf("port=%q", got)
	}
}
