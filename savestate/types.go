package savestate

type ErrorCode string

const (
	CodeFileTooLarge           ErrorCode = "FILE_TOO_LARGE"
	CodeNotZipSavestate        ErrorCode = "NOT_ZIP_SAVESTATE"
	CodeZipCorrupt             ErrorCode = "ZIP_CORRUPT"
	CodeDuplicateLump          ErrorCode = "DUPLICATE_LUMP"
	CodeMissingBizStateVersion ErrorCode = "MISSING_BIZSTATE_VERSION"
	CodeInvalidBizStateVersion ErrorCode = "INVALID_BIZSTATE_VERSION"
	CodeMissingCoreState       ErrorCode = "MISSING_CORE_STATE"
	CodeMissingBizVersion      ErrorCode = "MISSING_BIZ_VERSION"
	CodeEmuVersionMismatch     ErrorCode = "EMU_VERSION_MISMATCH"
	CodeMissingSyncSettings    ErrorCode = "MISSING_SYNC_SETTINGS"
	CodeSyncSettingsMismatch   ErrorCode = "SYNC_SETTINGS_MISMATCH"
	CodeMovieMismatch          ErrorCode = "MOVIE_MISMATCH"
	CodeCoreBlobSuspect        ErrorCode = "CORE_BLOB_SUSPECT"
)

type VerifyOptions struct {
	MaxFileBytes          int64
	ExpectedEmuVersion    string
	ExpectedSyncSettings  string
	ExpectedMovieInputLog []string
	SystemID              string
}

type VerifyResult struct {
	OK            bool
	Code          ErrorCode
	Message       string
	Detail        any
	FormatVersion string
	ZipSubVersion int
	Frame         *int
	HasCoreText   bool
}
