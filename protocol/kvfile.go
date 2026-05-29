package protocol

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// ForEachKVLine reads key=value lines from r and calls fn for each pair.
// Empty lines and lines without '=' are skipped. If lowercaseKeys is true, keys are lowercased.
// Returns the first error from fn or from scanner.Err() after the loop.
func ForEachKVLine(r io.Reader, lowercaseKeys bool, fn func(key, val string) error) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if lowercaseKeys {
			key = strings.ToLower(key)
		}
		if err := fn(key, val); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// ReadKVMap reads a kv file into a map (keys lowercased).
func ReadKVMap(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	out := make(map[string]string)
	err = ForEachKVLine(f, true, func(key, val string) error {
		if key != "" {
			out[key] = val
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
