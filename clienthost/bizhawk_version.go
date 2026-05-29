package clienthost

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// SupportedBizHawkVersion is the minimum BizHawk release this build supports.
const SupportedBizHawkVersion = "2.11.1"

var bizHawkDirVersionRe = regexp.MustCompile(`(?i)BizHawk[-_]?v?(\d+\.\d+(?:\.\d+)?)`)

// CompareBizHawkVersions compares dotted version strings (e.g. 2.9 vs 2.10).
func CompareBizHawkVersions(a, b string) int {
	pa := parseVersionParts(a)
	pb := parseVersionParts(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		va, vb := 0, 0
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

func parseVersionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}
	return out
}

// BizHawkNeedsUpdate reports whether installed is older than supported.
func BizHawkNeedsUpdate(installed, supported string) bool {
	if installed == "" {
		return false
	}
	return CompareBizHawkVersions(installed, supported) < 0
}

// DetectInstalledBizHawkVersion infers version from path or version.txt beside the exe.
func DetectInstalledBizHawkVersion(exePath string) string {
	normalized := filepath.ToSlash(exePath)
	for _, part := range strings.Split(normalized, "/") {
		if m := bizHawkDirVersionRe.FindStringSubmatch(part); len(m) > 1 {
			return m[1]
		}
	}
	versionFile := filepath.Join(filepath.Dir(exePath), "version.txt")
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return ""
	}
	m := regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)`).FindStringSubmatch(string(data))
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

// BizHawkStatus describes managed BizHawk under dataDir.
type BizHawkStatus struct {
	ExePath          string
	InstalledVersion string
	SupportedVersion string
	Missing          bool
	NeedsUpdate      bool
}
