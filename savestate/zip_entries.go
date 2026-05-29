package savestate

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const (
	lumpZipVersion = "BizState 1"
	lumpBizVersion = "BizVersion"
	lumpCoreBin    = "Core"
	lumpCoreText   = "CoreText"
	lumpSync       = "SyncSettings"
	lumpInput      = "Input Log"
)

type zipLump struct {
	relPath    string
	data       []byte
	compressed bool
}

func openZipArchive(fileBytes []byte) (map[string]zipLump, error) {
	r, err := zip.NewReader(bytes.NewReader(fileBytes), int64(len(fileBytes)))
	if err != nil {
		return nil, fmt.Errorf("ZIP_CORRUPT")
	}
	files := make(map[string][]byte)
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("ZIP_CORRUPT")
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("ZIP_CORRUPT")
		}
		files[f.Name] = data
	}
	return normalizeZipEntries(files)
}

func longestCommonPrefix(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	prefix := paths[0]
	for _, p := range paths[1:] {
		for len(prefix) > 0 && !strings.HasPrefix(p, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
		if prefix == "" {
			break
		}
	}
	if prefix != "" && !strings.HasSuffix(prefix, "/") && !strings.HasSuffix(prefix, "\\") {
		return ""
	}
	return prefix
}

func normalizeZipEntries(files map[string][]byte) (map[string]zipLump, error) {
	paths := make([]string, 0, len(files))
	for p := range files {
		if !strings.HasSuffix(p, "/") {
			paths = append(paths, p)
		}
	}
	commonPrefix := longestCommonPrefix(paths)
	result := make(map[string]zipLump)
	for fullName, data := range files {
		if strings.HasSuffix(fullName, "/") {
			continue
		}
		rel := fullName
		if commonPrefix != "" && strings.HasPrefix(rel, commonPrefix) {
			rel = rel[len(commonPrefix):]
		}
		rel = strings.ReplaceAll(rel, "\\", "/")
		dot := strings.Index(rel, ".")
		logicalBase := rel
		if dot >= 0 {
			logicalBase = rel[:dot]
		}
		logical := logicalBase
		if strings.HasSuffix(rel, ".zst") {
			logical = logicalBase + ".zst"
		}
		if _, exists := result[logical]; exists {
			return nil, fmt.Errorf("DUPLICATE_LUMP")
		}
		result[logical] = zipLump{
			relPath:    rel,
			data:       data,
			compressed: strings.HasSuffix(rel, ".zst"),
		}
	}
	return result, nil
}

func readLumpBytes(entries map[string]zipLump, logicalName, formatVersion string) ([]byte, error) {
	lump, ok := entries[logicalName]
	if !ok {
		lump, ok = entries[logicalName+".zst"]
	}
	if !ok {
		return nil, nil
	}
	ext := ""
	if i := strings.LastIndex(lump.relPath, "."); i >= 0 {
		ext = lump.relPath[i+1:]
	}
	if lump.compressed || isLegacyZstd(lump, formatVersion, ext) {
		dec, err := zstd.NewReader(nil)
		if err != nil {
			return nil, fmt.Errorf("ZSTD_DECOMPRESS_FAILED")
		}
		defer dec.Close()
		out, err := dec.DecodeAll(lump.data, nil)
		if err != nil {
			return nil, fmt.Errorf("ZSTD_DECOMPRESS_FAILED")
		}
		return out, nil
	}
	return lump.data, nil
}

func isLegacyZstd(lump zipLump, formatVersion, lumpExt string) bool {
	if lump.compressed {
		return true
	}
	if formatVersion != "1.0.2" {
		return false
	}
	if lumpExt == "bin" || lumpExt == "bmp" {
		return true
	}
	return lump.relPath == "Greenzone"
}

func readFirstLine(bytes []byte) string {
	text := string(bytes)
	line := strings.Split(text, "\n")[0]
	return strings.TrimSpace(line)
}

func readFirstNonEmptyLine(bytes []byte) string {
	for _, line := range strings.Split(string(bytes), "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

func parseInputLog(text string) (frame int, lines []string) {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "|") {
			lines = append(lines, line)
		} else if strings.HasPrefix(line, "Frame ") {
			var n int
			if _, err := fmt.Sscanf(line, "Frame %d", &n); err == nil && n > 0 {
				frame = n
			}
		}
	}
	if frame == 0 {
		frame = len(lines)
	}
	return frame, lines
}

func checkTimeline(movieLog, stateLog []string, stateFrame int) (bool, string) {
	if stateFrame > len(stateLog) {
		return false, "invalid frame number vs embedded log"
	}
	if len(movieLog) < stateFrame {
		return false, "state frame beyond movie length"
	}
	for i := 0; i < stateFrame; i++ {
		if movieLog[i] != stateLog[i] {
			return false, fmt.Sprintf("input mismatch at frame %d", i+1)
		}
	}
	return true, ""
}

func sanityCheckCoreBinary(raw []byte, systemID string) (bool, string) {
	if len(raw) < 4 {
		return false, "core blob too small"
	}
	coreLen := int32(binary.LittleEndian.Uint32(raw[:4]))
	if systemID == "GB" {
		if coreLen <= 0 || coreLen > 10_000_000 {
			return false, "implausible GB core length"
		}
	}
	if systemID == "N64" && coreLen < 16_788_288 {
		return false, "N64 core blob too small"
	}
	return true, ""
}
