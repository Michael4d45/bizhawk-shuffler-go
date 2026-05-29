package obslog

import (
	"net/url"
	"strconv"
	"strings"
)

// URLPort returns the TCP port from an http(s) URL, or "" if unknown.
func URLPort(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if u.Port() != "" {
		return u.Port()
	}
	switch u.Scheme {
	case "http", "ws":
		return "80"
	case "https", "wss":
		return "443"
	default:
		return ""
	}
}

// URLsSameHostPort reports whether two server base URLs target the same host:port.
func URLsSameHostPort(a, b string) bool {
	pa, pb := URLPort(a), URLPort(b)
	if pa == "" || pb == "" {
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}
	ua, errA := url.Parse(strings.TrimSpace(a))
	ub, errB := url.Parse(strings.TrimSpace(b))
	if errA != nil || errB != nil {
		return pa == pb
	}
	ha, hb := ua.Hostname(), ub.Hostname()
	if ha == "localhost" {
		ha = "127.0.0.1"
	}
	if hb == "localhost" {
		hb = "127.0.0.1"
	}
	return ha == hb && pa == pb
}

// WarnJoinHostPortMismatch logs when join URL port differs from the locally hosted server.
func WarnJoinHostPortMismatch(joinURL, hostedURL string) {
	if hostedURL == "" || joinURL == "" {
		return
	}
	if URLsSameHostPort(joinURL, hostedURL) {
		return
	}
	jp, hp := URLPort(joinURL), URLPort(hostedURL)
	Event(Join, "url_port_mismatch", map[string]string{
		"join_url":    joinURL,
		"hosted_url":  hostedURL,
		"join_port":   jp,
		"hosted_port": hp,
		"hint":        "set Server URL to hosted_url or use the discovered server list",
	})
}

// FormatIntPort formats an int port for event fields.
func FormatIntPort(port int) string {
	return strconv.Itoa(port)
}
