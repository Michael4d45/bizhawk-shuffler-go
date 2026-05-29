package clienthost

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/michael4d45/bizshuffle/protocol"
)

// DiscoveredServerEntry is one row for the desktop discovery list.
type DiscoveredServerEntry struct {
	Label    string
	URL      string
	IsHosted bool
}

func isLocalhostHost(host string) bool {
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

// ServerURLsMatch returns true when two server URLs refer to the same localhost port.
func ServerURLsMatch(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return strings.TrimSuffix(strings.TrimSpace(a), "/") == strings.TrimSuffix(strings.TrimSpace(b), "/")
	}
	if ua.Port() != ub.Port() {
		return false
	}
	return isLocalhostHost(ua.Hostname()) && isLocalhostHost(ub.Hostname())
}

// MergeDiscoveredServers builds discovery UI entries, marking the hosted session.
func MergeDiscoveredServers(servers []*protocol.ServerInfo, hostedURL, hostedLabel string) []DiscoveredServerEntry {
	var out []DiscoveredServerEntry
	hasHosted := false
	for _, s := range servers {
		entryURL := fmt.Sprintf("http://%s:%d", s.Host, s.Port)
		label := s.Name
		if label == "" {
			label = s.ServerID
		}
		isHosted := hostedURL != "" && ServerURLsMatch(entryURL, hostedURL)
		if isHosted {
			hasHosted = true
		}
		out = append(out, DiscoveredServerEntry{
			Label:    label,
			URL:      entryURL,
			IsHosted: isHosted,
		})
	}
	if hostedURL != "" && !hasHosted {
		label := hostedLabel
		if label == "" {
			label = "This session"
		}
		out = append([]DiscoveredServerEntry{{
			Label:    label,
			URL:      hostedURL,
			IsHosted: true,
		}}, out...)
	}
	return out
}
