package client

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// ProgressTracker tracks download progress and displays it in pacman style
type ProgressTracker struct {
	filename   string
	totalSize  int64
	downloaded int64
	startTime  time.Time
	lastUpdate time.Time
	width      int // width of progress bar
}

// NewProgressTracker creates a new progress tracker for a file download
func NewProgressTracker(filename string, totalSize int64) *ProgressTracker {
	return &ProgressTracker{
		filename:   filename,
		totalSize:  totalSize,
		downloaded: 0,
		startTime:  time.Now(),
		lastUpdate: time.Now(),
		width:      46, // matches pacman style from example
	}
}

// ProgressReader wraps an io.Reader to track download progress
type ProgressReader struct {
	reader  io.Reader
	tracker *ProgressTracker
}

// NewProgressReader creates a progress-tracking reader
func NewProgressReader(reader io.Reader, tracker *ProgressTracker) *ProgressReader {
	return &ProgressReader{
		reader:  reader,
		tracker: tracker,
	}
}

// Read implements io.Reader and updates progress
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.tracker.Update(int64(n))
	}
	return n, err
}

// Update increments the downloaded bytes and displays progress
func (pt *ProgressTracker) Update(bytes int64) {
	pt.downloaded += bytes
	now := time.Now()

	// Update display every 100ms to avoid flickering
	if now.Sub(pt.lastUpdate) >= 100*time.Millisecond || pt.downloaded >= pt.totalSize {
		pt.display()
		pt.lastUpdate = now
	}
}

// display shows the current progress in pacman style
func (pt *ProgressTracker) display() {
	// Calculate stats
	var percentage float64
	var speed float64
	var eta string

	if pt.totalSize > 0 {
		percentage = float64(pt.downloaded) / float64(pt.totalSize) * 100
	}

	elapsed := time.Since(pt.startTime).Seconds()
	if elapsed > 0 {
		speed = float64(pt.downloaded) / elapsed
	}

	if speed > 0 && pt.totalSize > pt.downloaded {
		remaining := float64(pt.totalSize-pt.downloaded) / speed
		eta = formatDuration(time.Duration(remaining) * time.Second)
	} else {
		eta = "00:00"
	}

	// Build progress bar
	filled := int(float64(pt.width) * percentage / 100)
	if filled > pt.width {
		filled = pt.width
	}

	bar := strings.Repeat("#", filled) + strings.Repeat("-", pt.width-filled)

	// Format file size
	sizeStr := formatBytes(pt.totalSize)
	speedStr := formatBytes(int64(speed)) + "/s"

	// Print progress line (overwrites previous line)
	fmt.Printf("\r %-50s %8s %10s %6s [%s] %3.0f%%",
		pt.filename,
		sizeStr,
		speedStr,
		eta,
		bar,
		percentage)

	// If complete, print newline
	if pt.downloaded >= pt.totalSize {
		fmt.Println()
	}
}

// Finish completes the progress display
func (pt *ProgressTracker) Finish() {
	if pt.downloaded < pt.totalSize {
		pt.downloaded = pt.totalSize
		pt.display()
	}
	fmt.Println()
}

// GetDownloaded returns the current downloaded bytes
func (pt *ProgressTracker) GetDownloaded() int64 {
	return pt.downloaded
}

// Error displays an error and moves to next line
func (pt *ProgressTracker) Error(err error) {
	fmt.Printf("\r %-50s ERROR: %v\n", pt.filename, err)
}

// formatBytes formats byte counts in human readable form
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// formatDuration formats duration in MM:SS format
func formatDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

// DownloadProgressManager manages multiple concurrent downloads
type DownloadProgressManager struct {
	activeDownloads map[string]*ProgressTracker
}

// NewDownloadProgressManager creates a new download progress manager
func NewDownloadProgressManager() *DownloadProgressManager {
	return &DownloadProgressManager{
		activeDownloads: make(map[string]*ProgressTracker),
	}
}

// StartDownload begins tracking a new download
func (dpm *DownloadProgressManager) StartDownload(filename string, totalSize int64) *ProgressTracker {
	tracker := NewProgressTracker(filename, totalSize)
	dpm.activeDownloads[filename] = tracker
	return tracker
}

// FinishDownload completes tracking for a download
func (dpm *DownloadProgressManager) FinishDownload(filename string) {
	if tracker, exists := dpm.activeDownloads[filename]; exists {
		tracker.Finish()
		delete(dpm.activeDownloads, filename)
	}
}

// ErrorDownload marks a download as errored
func (dpm *DownloadProgressManager) ErrorDownload(filename string, err error) {
	if tracker, exists := dpm.activeDownloads[filename]; exists {
		tracker.Error(err)
		delete(dpm.activeDownloads, filename)
	}
}

// Global progress manager instance
var globalProgressManager = NewDownloadProgressManager()
