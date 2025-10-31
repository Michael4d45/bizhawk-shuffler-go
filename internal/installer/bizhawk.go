package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// GetBizHawkDownloadURL returns the default BizHawk download URL for the current platform
func GetBizHawkDownloadURL() string {
	// Using the same URL as in client config.go
	return "https://github.com/TASEmulators/BizHawk/releases/download/2.10/BizHawk-2.10-win-x64.zip"
}
