// Package obslog provides tagged logging and a machine-readable trace for desktop debugging.
package obslog

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Component identifies which part of the desktop app emitted a log line.
type Component string

const (
	Session   Component = "session"
	Host      Component = "host"
	Join      Component = "join"
	WS        Component = "ws"
	Lua       Component = "lua"
	Swap Component = "swap"
)

var (
	mu        sync.Mutex
	traceFile *os.File
	sessionID string
)

// Init starts a new observability session. It writes a banner to desktop.log and opens
// dataDir/debug-trace.ndjson for structured events (one JSON object per line).
func Init(dataDir string) error {
	sessionID = time.Now().Format("20060102-150405")
	Separator("app start")

	if dataDir == "" {
		return nil
	}
	path := filepath.Join(dataDir, "debug-trace.ndjson")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("[session] debug trace unavailable: %v", err)
		return err
	}
	mu.Lock()
	if traceFile != nil {
		_ = traceFile.Close()
	}
	traceFile = f
	mu.Unlock()

	Event(Session, "init", map[string]string{
		"trace_file": path,
		"session_id": sessionID,
	})
	return nil
}

// SessionID returns the id for the current desktop run (set by Init).
func SessionID() string {
	return sessionID
}

// Close releases the debug trace file handle.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if traceFile != nil {
		_ = traceFile.Close()
		traceFile = nil
	}
}

// Separator writes a visible break between runs in desktop.log.
func Separator(label string) {
	log.Printf("[session] ========== %s (session=%s) ==========", label, sessionID)
}

// Print logs with a fixed component prefix.
func Print(c Component, format string, args ...any) {
	log.Printf("[%s] "+format, append([]any{c}, args...)...)
}

// Event logs a named event to desktop.log and appends one NDJSON record to debug-trace.ndjson.
func Event(c Component, name string, fields map[string]string) {
	if fields == nil {
		fields = map[string]string{}
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, " %s=%q", k, fields[k])
	}
	log.Printf("[%s] event=%s%s", c, name, b.String())

	record := map[string]any{
		"ts":        time.Now().Format(time.RFC3339Nano),
		"session":   sessionID,
		"component": string(c),
		"event":     name,
	}
	for k, v := range fields {
		record[k] = v
	}
	writeNDJSON(record)
}

func writeNDJSON(record map[string]any) {
	mu.Lock()
	f := traceFile
	mu.Unlock()
	if f == nil {
		return
	}
	line, err := json.Marshal(record)
	if err != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if traceFile == nil {
		return
	}
	_, _ = traceFile.Write(append(line, '\n'))
}
