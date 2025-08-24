package server

// maxSaveSize is the maximum allowed size for uploaded save files (in bytes).
// Keep this conservative to avoid excessive memory/disk usage from uploads.
const maxSaveSize int64 = 10 << 20 // 10 MiB
