package hostsession

import (
	"os"
	"testing"
)

func TestLocalAdminURL(t *testing.T) {
	if u := LocalAdminURL("0.0.0.0", 8080); u != "http://127.0.0.1:8080/" {
		t.Fatalf("got %q", u)
	}
	if u := LocalAdminURL("127.0.0.1", 9090); u != "http://127.0.0.1:9090/" {
		t.Fatalf("got %q", u)
	}
}

func TestNormalizeBindHost(t *testing.T) {
	h, err := NormalizeBindHost("")
	if err != nil || h != "127.0.0.1" {
		t.Fatalf("got %q %v", h, err)
	}
	if _, err := NormalizeBindHost("not valid!"); err == nil {
		t.Fatal("expected error")
	}
}

func TestStartPortZero(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	var sess Session
	res, err := sess.Start(t.Context(), "127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Stop() })
	if res.HostPort <= 0 || res.HostPort > 65535 {
		t.Fatalf("port %d", res.HostPort)
	}
}
