package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// BizHawkInstaller handles BizHawk emulator installation
type BizHawkInstaller struct {
	downloader *Downloader
}

// NewBizHawkInstaller creates a new BizHawk installer
func NewBizHawkInstaller() *BizHawkInstaller {
	return &BizHawkInstaller{
		downloader: NewDownloader(),
	}
}

// InstallBizHawk downloads and installs BizHawk to the specified directory
func (b *BizHawkInstaller) InstallBizHawk(downloadURL, installDir string, progress func(msg string)) error {
	if progress != nil {
		progress("Downloading BizHawk...")
	}

	archiveFile := filepath.Base(downloadURL)
	archivePath := filepath.Join(os.TempDir(), archiveFile)

	if err := b.downloader.DownloadFile(downloadURL, archivePath, nil); err != nil {
		return fmt.Errorf("failed to download BizHawk: %w", err)
	}
	defer func() { _ = os.Remove(archivePath) }()

	if progress != nil {
		progress("Extracting BizHawk...")
	}

	// Determine archive type and extract
	if strings.HasSuffix(strings.ToLower(downloadURL), ".zip") {
		if err := b.extractZip(archivePath, installDir); err != nil {
			return fmt.Errorf("failed to extract BizHawk: %w", err)
		}
	} else if strings.HasSuffix(strings.ToLower(downloadURL), ".tar.gz") || strings.HasSuffix(strings.ToLower(downloadURL), ".tgz") {
		if err := b.extractTarGz(archivePath, installDir); err != nil {
			return fmt.Errorf("failed to extract BizHawk: %w", err)
		}
	} else {
		// Try zip as fallback
		if err := b.extractZip(archivePath, installDir); err != nil {
			return fmt.Errorf("failed to extract BizHawk (unknown archive): %w", err)
		}
	}

	if progress != nil {
		progress("BizHawk installation complete")
	}
	return nil
}

// ExtractZip extracts a zip file to the destination directory
func (b *BizHawkInstaller) ExtractZip(zipPath, destDir string) error {
	return b.extractZip(zipPath, destDir)
}

func (b *BizHawkInstaller) extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, f.Mode()); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			_ = outFile.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		if cerr := outFile.Close(); cerr != nil {
			_ = rc.Close()
			return cerr
		}
		if cerr := rc.Close(); cerr != nil {
			return cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *BizHawkInstaller) extractTarGz(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		fpath := filepath.Join(destDir, hdr.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(fpath, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(fpath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				_ = outFile.Close()
				return err
			}
			if err := outFile.Close(); err != nil {
				return err
			}
		default:
			// ignore other types
		}
	}
	return nil
}

const (
	bizhawkRepoOwner = "TASEmulators"
	bizhawkRepoName  = "BizHawk"
	bizhawkAPIURL    = "https://api.github.com"
)

// GetBizHawkPlatformSuffix returns the platform suffix for BizHawk asset names
func GetBizHawkPlatformSuffix() string {
	switch runtime.GOOS {
	case "windows":
		switch runtime.GOARCH {
		case "amd64", "386":
			return "win-x64"
		default:
			return "win-x64" // Default to x64 for Windows
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return "linux-x64"
		case "arm64":
			return "linux-arm64"
		default:
			return "linux-x64" // Default to x64 for Linux
		}
	case "darwin":
		switch runtime.GOARCH {
		case "amd64":
			return "osx-x64"
		case "arm64":
			return "osx-arm64"
		default:
			return "osx-x64" // Default to x64 for macOS
		}
	default:
		return "win-x64" // Fallback to Windows x64
	}
}

// GetBizHawkReleaseByTag fetches a BizHawk release by tag (e.g. 2.11.1).
func GetBizHawkReleaseByTag(tag string) (*Release, error) {
	tag = strings.TrimPrefix(tag, "v")
	url := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", bizhawkAPIURL, bizhawkRepoOwner, bizhawkRepoName, tag)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release JSON: %w", err)
	}
	return &release, nil
}

// GetBizHawkLatestRelease fetches the latest BizHawk release from GitHub
func GetBizHawkLatestRelease() (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", bizhawkAPIURL, bizhawkRepoOwner, bizhawkRepoName)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release JSON: %w", err)
	}

	return &release, nil
}

// GetBizHawkDownloadURLForVersion returns the download URL for a pinned BizHawk version.
func GetBizHawkDownloadURLForVersion(version string) (string, error) {
	release, err := GetBizHawkReleaseByTag(version)
	if err != nil {
		return fallbackBizHawkDownloadURL(version), nil
	}
	platformSuffix := GetBizHawkPlatformSuffix()
	tagName := strings.TrimPrefix(release.TagName, "v")
	patterns := []string{
		fmt.Sprintf("BizHawk-%s-%s.zip", tagName, platformSuffix),
		fmt.Sprintf("BizHawk-%s-%s.zip", release.TagName, platformSuffix),
	}
	for _, pattern := range patterns {
		if asset := release.FindAssetByName(pattern); asset != nil {
			return asset.DownloadURL, nil
		}
	}
	for _, a := range release.Assets {
		if strings.Contains(a.Name, platformSuffix) && strings.HasSuffix(a.Name, ".zip") {
			return a.DownloadURL, nil
		}
	}
	return fallbackBizHawkDownloadURL(version), nil
}

func fallbackBizHawkDownloadURL(version string) string {
	tag := strings.TrimPrefix(version, "v")
	suffix := GetBizHawkPlatformSuffix()
	return fmt.Sprintf("https://github.com/TASEmulators/BizHawk/releases/download/%s/BizHawk-%s-%s.zip", tag, tag, suffix)
}

// GetBizHawkDownloadURL returns the download URL for the supported pinned version.
func GetBizHawkDownloadURL() string {
	url, err := GetBizHawkDownloadURLForVersion("2.11.1")
	if err != nil {
		return fallbackBizHawkDownloadURL("2.11.1")
	}
	return url
}
