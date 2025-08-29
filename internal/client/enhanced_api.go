package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// EnhancedAPI extends the API with progress tracking and extra files support
type EnhancedAPI struct {
	*API
	controller *Controller
}

// NewEnhancedAPI creates an enhanced API that can access controller state
func NewEnhancedAPI(api *API, controller *Controller) *EnhancedAPI {
	return &EnhancedAPI{
		API:        api,
		controller: controller,
	}
}

// EnsureFileWithProgress ensures the named file exists locally, downloading it with progress display.
// Also downloads any extra_files associated with the main file if available.
func (ea *EnhancedAPI) EnsureFileWithProgress(ctx context.Context, name string) error {
	// First ensure the main file
	if err := ea.ensureFileWithProgressInternal(ctx, name); err != nil {
		return err
	}

	// If this is a main game file, also ensure extra files
	if ea.controller != nil {
		extraFiles := ea.controller.GetExtraFilesForGame(name)
		for _, extra := range extraFiles {
			if err := ea.ensureFileWithProgressInternal(ctx, extra); err != nil {
				return fmt.Errorf("failed to download extra file %s: %w", extra, err)
			}
		}
	}

	return nil
}

// ensureFileWithProgressInternal downloads a single file with progress tracking
func (ea *EnhancedAPI) ensureFileWithProgressInternal(ctx context.Context, name string) error {
	dest := filepath.Join("./roms", filepath.FromSlash(name))
	if _, err := os.Stat(dest); err == nil {
		return nil // exists
	}

	// ensure directory
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}

	// build URL
	fetch := ea.BaseURL
	if len(fetch) > 0 && fetch[len(fetch)-1] == '/' {
		fetch = fetch[:len(fetch)-1]
	}
	fetch += "/files/" + name

	// try up to 3 times
	var lastErr error
	for i := 0; i < 3; i++ {
		if err := ea.downloadFileWithProgress(ctx, fetch, dest, name); err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		return nil
	}
	return lastErr
}

// downloadFileWithProgress downloads a file with progress tracking
func (ea *EnhancedAPI) downloadFileWithProgress(ctx context.Context, url, dest, displayName string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := ea.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Get content length for progress tracking
	contentLength := resp.ContentLength

	// Start progress tracking
	tracker := globalProgressManager.StartDownload(displayName, contentLength)
	defer globalProgressManager.FinishDownload(displayName)

	// Create progress reader
	progressReader := NewProgressReader(resp.Body, tracker)

	// Create output file
	out, err := os.Create(dest)
	if err != nil {
		globalProgressManager.ErrorDownload(displayName, err)
		return err
	}
	defer func() { _ = out.Close() }()

	// Copy with progress tracking
	_, err = io.Copy(out, progressReader)
	if err != nil {
		globalProgressManager.ErrorDownload(displayName, err)
		return err
	}

	return nil
}
