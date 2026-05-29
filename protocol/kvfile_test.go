package protocol

import (
	"strings"
	"testing"
)

func TestForEachKVLine_scannerErr(t *testing.T) {
	// Line longer than default max token size triggers scanner.Err() after Scan returns false.
	long := strings.Repeat("x", bufioMaxScanTokenSize+1)
	r := strings.NewReader("ok=1\n" + long + "\n")
	var keys []string
	err := ForEachKVLine(r, true, func(key, _ string) error {
		keys = append(keys, key)
		return nil
	})
	if err == nil {
		t.Fatal("expected scanner error for oversized line")
	}
	if len(keys) != 1 || keys[0] != "ok" {
		t.Fatalf("keys before error = %v, want [ok]", keys)
	}
}

// bufioMaxScanTokenSize is 64*1024 in the stdlib; mirror for test input size.
const bufioMaxScanTokenSize = 64 * 1024
