package protocol

import (
	"regexp"
	"sort"
	"strings"
)

func ParseKv(content string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(trimmed[:eq]))
		value := strings.TrimSpace(trimmed[eq+1:])
		result[key] = value
	}
	return result
}

func WriteKv(entries map[string]string, statusFirst bool) string {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	if statusFirst && entries["status"] != "" {
		var b strings.Builder
		writeKvLine(&b, "status", entries["status"])
		rest := make([]string, 0, len(keys))
		for _, k := range keys {
			if k != "status" {
				rest = append(rest, k)
			}
		}
		sort.Strings(rest)
		for _, k := range rest {
			writeKvLine(&b, k, entries[k])
		}
		return b.String()
	}
	sort.Strings(keys)
	var lines []string
	for _, k := range keys {
		lines = append(lines, k+"="+entries[k])
	}
	return strings.Join(lines, "\n") + "\n"
}

func writeKvLine(b *strings.Builder, key, val string) {
	b.WriteString(key)
	b.WriteString("=")
	b.WriteString(val)
	b.WriteString("\n")
}

var settingMetaRe = regexp.MustCompile(`^setting\.([^.]+)\.(type|options)$`)

func ParseSettingsMeta(meta map[string]string) map[string]SettingMeta {
	result := make(map[string]SettingMeta)
	for key, value := range meta {
		m := settingMetaRe.FindStringSubmatch(key)
		if m == nil {
			continue
		}
		settingKey := m[1]
		field := m[2]
		entry, ok := result[settingKey]
		if !ok {
			entry = SettingMeta{Type: "text"}
		}
		if field == "type" {
			entry.Type = value
		} else {
			var opts []string
			for _, part := range strings.Split(value, ",") {
				p := strings.TrimSpace(part)
				if p != "" {
					opts = append(opts, p)
				}
			}
			entry.Options = opts
		}
		result[settingKey] = entry
	}
	return result
}
