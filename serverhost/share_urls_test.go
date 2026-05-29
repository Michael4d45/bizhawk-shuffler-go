package serverhost

import (
	"context"
	"testing"
)

func TestIsLocalOnlyBind(t *testing.T) {
	if !IsLocalOnlyBind("127.0.0.1") {
		t.Fatal("expected loopback local only")
	}
	if len(BuildLANShareURLs("127.0.0.1", 8080)) != 0 {
		t.Fatal("expected no LAN urls for loopback")
	}
}

func TestBuildLANShareURLsExplicitHost(t *testing.T) {
	got := BuildLANShareURLs("192.168.1.10", 8080)
	if len(got) != 1 || got[0] != "http://192.168.1.10:8080" {
		t.Fatalf("got %v", got)
	}
}

func TestResolveShareURLsWAN(t *testing.T) {
	fetch := func(ctx context.Context) (string, error) {
		return "203.0.113.5", nil
	}
	urls, err := ResolveShareURLs(context.Background(), "0.0.0.0", 9090, fetch)
	if err != nil {
		t.Fatal(err)
	}
	if urls.LocalOnly {
		t.Fatal("expected not local only")
	}
	if urls.WAN == nil || *urls.WAN != "http://203.0.113.5:9090" {
		t.Fatalf("wan %+v", urls.WAN)
	}
}

func TestResolveShareURLsLoopbackSkipsWAN(t *testing.T) {
	fetch := func(ctx context.Context) (string, error) {
		return "203.0.113.5", nil
	}
	urls, err := ResolveShareURLs(context.Background(), "127.0.0.1", 8080, fetch)
	if err != nil {
		t.Fatal(err)
	}
	if !urls.LocalOnly || urls.WAN != nil {
		t.Fatalf("got %+v", urls)
	}
}

func TestDiscoveryAdvertiseHostWildcard(t *testing.T) {
	host := DiscoveryAdvertiseHost("0.0.0.0")
	if host == "0.0.0.0" {
		t.Fatal("expected concrete advertise host")
	}
	if IsLocalOnlyBind(host) {
		t.Fatal("expected non-loopback advertise host when possible")
	}
}
