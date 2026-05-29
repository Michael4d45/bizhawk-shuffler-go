package savestate

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strconv"
)

const defaultMaxBytes = 64 * 1024 * 1024

var zipMagic = []byte{0x50, 0x4b, 0x03, 0x04}

func fail(code ErrorCode, message string, detail any) VerifyResult {
	return VerifyResult{OK: false, Code: code, Message: message, Detail: detail}
}

func hasZipMagic(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	for i := range zipMagic {
		if b[i] != zipMagic[i] {
			return false
		}
	}
	return true
}

func IsProbablyBizHawkSavestate(input []byte) bool {
	if !hasZipMagic(input) {
		return false
	}
	entries, err := openZipArchive(input)
	if err != nil {
		return false
	}
	hasVersion := mapHas(entries, lumpZipVersion) || mapHas(entries, lumpZipVersion+".zst")
	hasCore := mapHas(entries, lumpCoreBin) || mapHas(entries, lumpCoreBin+".zst") ||
		mapHas(entries, lumpCoreText) || mapHas(entries, lumpCoreText+".zst")
	return hasVersion && hasCore
}

func mapHas(m map[string]zipLump, key string) bool {
	_, ok := m[key]
	return ok
}

func VerifyBizHawkSavestate(input []byte, opts VerifyOptions) VerifyResult {
	maxBytes := opts.MaxFileBytes
	if maxBytes == 0 {
		maxBytes = defaultMaxBytes
	}
	if int64(len(input)) > maxBytes {
		return fail(CodeFileTooLarge, fmt.Sprintf("save exceeds %d bytes", maxBytes), nil)
	}
	if !hasZipMagic(input) {
		return fail(CodeNotZipSavestate, "file is not a ZIP-based BizHawk savestate", nil)
	}
	entries, err := openZipArchive(input)
	if err != nil {
		if err.Error() == "DUPLICATE_LUMP" {
			return fail(CodeDuplicateLump, "duplicate lump in savestate archive", nil)
		}
		return fail(CodeZipCorrupt, "savestate ZIP archive is corrupt or unreadable", nil)
	}
	if !mapHas(entries, lumpZipVersion) && !mapHas(entries, lumpZipVersion+".zst") {
		return fail(CodeMissingBizStateVersion, `missing "BizState 1" version lump`, nil)
	}
	formatVersion := "1.0.0"
	zipSubVersion := 0
	verBytes, err := readLumpBytes(entries, lumpZipVersion, formatVersion)
	if err != nil {
		if err.Error() == "ZSTD_DECOMPRESS_FAILED" {
			return fail(CodeZipCorrupt, "failed to decompress BizState version lump", nil)
		}
		return fail(CodeZipCorrupt, err.Error(), nil)
	}
	if verBytes == nil {
		return fail(CodeMissingBizStateVersion, `missing "BizState 1" version lump`, nil)
	}
	n, err := strconv.Atoi(readFirstLine(verBytes))
	if err != nil {
		return fail(CodeInvalidBizStateVersion, "BizState version lump is not an integer", nil)
	}
	zipSubVersion = n
	formatVersion = fmt.Sprintf("1.0.%d", zipSubVersion)

	hasCoreBin := mapHas(entries, lumpCoreBin) || mapHas(entries, lumpCoreBin+".zst")
	hasCoreText := mapHas(entries, lumpCoreText) || mapHas(entries, lumpCoreText+".zst")
	if !hasCoreBin && !hasCoreText {
		return fail(CodeMissingCoreState, "missing Core.bin and CoreText lumps", nil)
	}

	if opts.ExpectedEmuVersion != "" {
		bizVer, err := readLumpBytes(entries, lumpBizVersion, formatVersion)
		if err != nil {
			if err.Error() == "ZSTD_DECOMPRESS_FAILED" {
				return fail(CodeZipCorrupt, "failed to decompress BizVersion lump", nil)
			}
			return fail(CodeZipCorrupt, err.Error(), nil)
		}
		if bizVer == nil {
			return fail(CodeMissingBizVersion, "missing BizVersion.txt lump", nil)
		}
		emuVer := readFirstLine(bizVer)
		if emuVer != opts.ExpectedEmuVersion {
			return fail(CodeEmuVersionMismatch, "emulator version mismatch", map[string]string{"got": emuVer})
		}
	}

	if opts.ExpectedSyncSettings != "" {
		syncBytes, err := readLumpBytes(entries, lumpSync, formatVersion)
		if err != nil {
			if err.Error() == "ZSTD_DECOMPRESS_FAILED" {
				return fail(CodeZipCorrupt, "failed to decompress SyncSettings lump", nil)
			}
			return fail(CodeZipCorrupt, err.Error(), nil)
		}
		if syncBytes == nil {
			return fail(CodeMissingSyncSettings, "missing SyncSettings.json lump", nil)
		}
		if readFirstNonEmptyLine(syncBytes) != opts.ExpectedSyncSettings {
			return fail(CodeSyncSettingsMismatch, "sync settings JSON mismatch", nil)
		}
	}

	var frame *int
	if mapHas(entries, lumpInput) || mapHas(entries, lumpInput+".zst") {
		inputBytes, err := readLumpBytes(entries, lumpInput, formatVersion)
		if err != nil {
			if err.Error() == "ZSTD_DECOMPRESS_FAILED" {
				return fail(CodeZipCorrupt, "failed to decompress Input Log lump", nil)
			}
			return fail(CodeZipCorrupt, err.Error(), nil)
		}
		if inputBytes != nil {
			f, lines := parseInputLog(string(inputBytes))
			frame = &f
			if len(opts.ExpectedMovieInputLog) > 0 {
				ok, detail := checkTimeline(opts.ExpectedMovieInputLog, lines, f)
				if !ok {
					return fail(CodeMovieMismatch, "input log does not match expected movie", detail)
				}
			}
		}
	}

	if hasCoreBin && opts.SystemID != "" {
		coreBytes, err := readLumpBytes(entries, lumpCoreBin, formatVersion)
		if err != nil {
			if err.Error() == "ZSTD_DECOMPRESS_FAILED" {
				return fail(CodeZipCorrupt, "failed to decompress Core lump", nil)
			}
			return fail(CodeZipCorrupt, err.Error(), nil)
		}
		if coreBytes != nil {
			ok, detail := sanityCheckCoreBinary(coreBytes, opts.SystemID)
			if !ok {
				return fail(CodeCoreBlobSuspect, "core binary failed sanity check", detail)
			}
		}
	}

	return VerifyResult{
		OK: true, FormatVersion: formatVersion, ZipSubVersion: zipSubVersion,
		Frame: frame, HasCoreText: hasCoreText,
	}
}

// BuildNonBizHawkZip returns a valid ZIP that is not a BizHawk savestate.
func BuildNonBizHawkZip() ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("readme.txt")
	if err != nil {
		return nil, err
	}
	if _, err := f.Write([]byte("not a savestate")); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func BuildMinimalBizHawkSavestate() ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	add := func(name string, data []byte) error {
		f, err := w.Create(name)
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	}
	if err := add("BizState/BizState 1.0", []byte("3\n")); err != nil {
		return nil, err
	}
	if err := add("BizState/Core.bin", []byte{4, 0, 0, 0, 0}); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var InvalidSaveZip = []byte{
	0x50, 0x4b, 0x05, 0x06, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}
