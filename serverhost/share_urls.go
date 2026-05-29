package serverhost

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ShareUrls is returned by GET /api/share_urls for the admin share panel.
type ShareUrls struct {
	LAN       []string `json:"lan"`
	WAN       *string  `json:"wan"`
	LocalOnly bool     `json:"local_only"`
}

// PublicIPFetcher resolves the machine's public IPv4 address for WAN share URLs.
type PublicIPFetcher func(ctx context.Context) (string, error)

func isLoopbackHost(host string) bool {
	h := strings.ToLower(host)
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

func isWildcardBind(host string) bool {
	return host == "0.0.0.0" || host == "::" || host == "[::]"
}

func lanIPv4Addresses() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	ips := make(map[string]struct{})
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() {
				continue
			}
			ips[ipNet.IP.String()] = struct{}{}
		}
	}
	out := make([]string, 0, len(ips))
	for ip := range ips {
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

func joinShareURL(host string, port int) string {
	portStr := strconv.Itoa(port)
	if strings.Contains(host, ":") {
		return "http://[" + host + "]:" + portStr
	}
	return "http://" + host + ":" + portStr
}

// BuildLANShareURLs returns HTTP URLs for LAN clients based on bind host.
func BuildLANShareURLs(listenHost string, port int) []string {
	if isLoopbackHost(listenHost) {
		return nil
	}
	if isWildcardBind(listenHost) {
		ips := lanIPv4Addresses()
		out := make([]string, 0, len(ips))
		for _, ip := range ips {
			out = append(out, joinShareURL(ip, port))
		}
		return out
	}
	return []string{joinShareURL(listenHost, port)}
}

// IsLocalOnlyBind reports whether the bind address is loopback-only.
func IsLocalOnlyBind(listenHost string) bool {
	return isLoopbackHost(listenHost)
}

// ResolveShareURLs builds LAN/WAN share URLs for the admin panel.
func ResolveShareURLs(ctx context.Context, listenHost string, port int, fetchPublicIP PublicIPFetcher) (ShareUrls, error) {
	if fetchPublicIP == nil {
		fetchPublicIP = defaultFetchPublicIP
	}
	localOnly := IsLocalOnlyBind(listenHost)
	lan := BuildLANShareURLs(listenHost, port)
	var wan *string
	if !localOnly {
		ip, err := fetchPublicIP(ctx)
		if err == nil && ip != "" {
			u := joinShareURL(ip, port)
			wan = &u
		}
	}
	return ShareUrls{LAN: lan, WAN: wan, LocalOnly: localOnly}, nil
}

func defaultFetchPublicIP(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org?format=json", nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", nil
	}
	var body struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return strings.TrimSpace(body.IP), nil
}

func (s *Server) apiShareURLs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	host := s.PersistedHost()
	if host == "" {
		host = "127.0.0.1"
	}
	port := s.PersistedPort()
	if port == 0 {
		port = 8080
	}
	urls, err := ResolveShareURLs(r.Context(), host, port, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(urls)
}
