package client

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// DownloadFile downloads a URL to the given destination path.
func DownloadFile(client *http.Client, url, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, resp.Body)
	return err
}

// DownloadAndExtractZip downloads a zip file and extracts it into destDir.
func DownloadAndExtractZip(client *http.Client, url, zipPath, destDir string) error {
	if err := DownloadFile(client, url, zipPath); err != nil {
		return err
	}
	defer func() { _ = os.Remove(zipPath) }()
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
		if err := os.MkdirAll(filepath.Dir(fpath), 0o755); err != nil {
			return err
		}
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			if err := outFile.Close(); err != nil {
				return err
			}
			return err
		}
		_, err = io.Copy(outFile, rc)
		if err := outFile.Close(); err != nil {
			if err := rc.Close(); err != nil {
				return err
			}
			return err
		}
		if err := rc.Close(); err != nil {
			return err
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// DownloadAndExtractTarGz downloads a tar.gz (or tgz) file and extracts it into destDir.
func DownloadAndExtractTarGz(client *http.Client, url, tarPath, destDir string) error {
	if err := DownloadFile(client, url, tarPath); err != nil {
		return err
	}
	defer func() { _ = os.Remove(tarPath) }()

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
		// sanitize and join
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
			if err := os.MkdirAll(filepath.Dir(fpath), 0o755); err != nil {
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
			// ignore other types (symlinks, etc.) for now
		}
	}
	return nil
}

// EnsureBizHawkInstalled checks for BizHawk at cfgPath and downloads/extracts it if missing.
func EnsureBizHawkInstalled(httpClient *http.Client, cfg map[string]string) error {
	// cfg may contain keys: bizhawk_download_url, bizhawk_path
	// normalize alternate keys
	if v, ok := cfg["BizHawkDownloadURL"]; ok && cfg["bizhawk_download_url"] == "" {
		cfg["bizhawk_download_url"] = v
	}
	if v, ok := cfg["BizHawkPath"]; ok && cfg["bizhawk_path"] == "" {
		cfg["bizhawk_path"] = v
	}

	downloadURL := cfg["bizhawk_download_url"]
	// If no download URL was provided, but a path is configured, trust the path.
	if downloadURL == "" {
		if p := cfg["bizhawk_path"]; strings.TrimSpace(p) != "" {
			// nothing to do, path provided
			return nil
		}
		// No download URL and no path -> error: BizHawk required
		return fmt.Errorf("bizhawk not configured: provide bizhawk_download_url or bizhawk_path in config")
	}
	zipFile := filepath.Base(downloadURL)
	// compute installDir based on extension: .zip -> name without .zip, .tar.gz/.tgz -> name without .tar.gz
	installDir := strings.TrimSuffix(zipFile, filepath.Ext(zipFile))
	if strings.HasSuffix(zipFile, ".tar.gz") || strings.HasSuffix(zipFile, ".tgz") {
		// remove .tar.gz / .tgz
		installDir = strings.TrimSuffix(installDir, ".tar")
	}

	// Extra debug: report cwd and intended installDir
	if wd, err := os.Getwd(); err == nil {
		log.Printf("EnsureBizHawkInstalled: cwd=%s downloadURL=%s zipFile=%s installDir=%s expectedCfgPath=%s", wd, downloadURL, zipFile, installDir, cfg["bizhawk_path"])
	} else {
		log.Printf("EnsureBizHawkInstalled: Getwd failed: %v", err)
	}

	// choose expected exe path inside installDir, prefer OS-specific name
	expected := cfg["bizhawk_path"]
	if expected == "" {
		if runtime.GOOS == "windows" {
			expected = filepath.Join(installDir, "EmuHawk.exe")
		} else {
			expected = filepath.Join(installDir, "EmuHawkMono.sh")
		}
		cfg["bizhawk_path"] = expected
	}

	if _, err := os.Stat(expected); os.IsNotExist(err) {
		log.Printf("BizHawk not found at %s, downloading...", expected)
		// pick extractor based on URL
		if strings.HasSuffix(strings.ToLower(downloadURL), ".zip") {
			if err := DownloadAndExtractZip(httpClient, downloadURL, zipFile, installDir); err != nil {
				return fmt.Errorf("failed to download/extract BizHawk: %w", err)
			}
		} else if strings.HasSuffix(strings.ToLower(downloadURL), ".tar.gz") || strings.HasSuffix(strings.ToLower(downloadURL), ".tgz") {
			if err := DownloadAndExtractTarGz(httpClient, downloadURL, zipFile, installDir); err != nil {
				return fmt.Errorf("failed to download/extract BizHawk: %w", err)
			}
		} else {
			// unknown archive type: attempt zip first
			if err := DownloadAndExtractZip(httpClient, downloadURL, zipFile, installDir); err != nil {
				return fmt.Errorf("failed to download/extract BizHawk (unknown archive): %w", err)
			}
		}
		// optional: look for additional files from server
		if server, ok := cfg["server"]; ok && server != "" {
			bizFilesURL := strings.TrimSuffix(server, "/") + "/api/BizhawkFiles.zip"
			if err := DownloadAndExtractZip(httpClient, bizFilesURL, "BizhawkFiles.zip", installDir); err != nil {
				log.Printf("warning: failed to download BizhawkFiles.zip: %v", err)
			}
		} else {
			log.Printf("no server configured, skipping BizhawkFiles.zip download")
		}
		// After extraction, list installDir contents for debugging
		log.Printf("BizHawk installed into %s", installDir)
		if entries, err := os.ReadDir(installDir); err == nil {
			for _, e := range entries {
				info, _ := e.Info()
				log.Printf(" - %s (dir=%v mode=%v)", e.Name(), e.IsDir(), info.Mode())
			}
		} else {
			log.Printf("EnsureBizHawkInstalled: failed to read installDir %s: %v", installDir, err)
		}
		// persist the computed path in cfg (caller should save cfg)
		cfg["bizhawk_path"] = expected
	}
	return nil
}

// LaunchBizHawk starts the BizHawk executable with environment variables and returns the *exec.Cmd.
func LaunchBizHawk(ctx context.Context, cfg map[string]string, httpClient *http.Client) (*exec.Cmd, error) {
	// normalize alternate keys
	if v, ok := cfg["BizHawkPath"]; ok && cfg["bizhawk_path"] == "" {
		cfg["bizhawk_path"] = v
	}

	bp := cfg["bizhawk_path"]
	if strings.TrimSpace(bp) == "" {
		return nil, fmt.Errorf("bizhawk_path not configured")
	}
	// Try to resolve relative paths before failing: if bp is not absolute
	// and doesn't exist as given, look relative to the running executable
	// directory and also under ./bin/client/ which is a common layout for
	// the distributed client bundle. This helps when cfg contains a
	// relative path like "BizHawk-2.10-win-x64\\EmuHawk.exe".
	// If the configured path doesn't exist, try several resolution strategies:
	// 1) If bp is absolute, leave it (we'll check existence below)
	// 2) If bp is relative, try: a) next to running executable, b) ./bin/client/<bp>
	// 3) Try exec.LookPath to see if it's on PATH
	// 4) As a last resort, try join with current working dir.
	if _, err := os.Stat(bp); os.IsNotExist(err) {
		log.Printf("Debug: initial bizhawk path %q does not exist from cwd %s", bp, func() string {
			if wd, e := os.Getwd(); e == nil {
				return wd
			} else {
				return "<getwd error>"
			}
		}())
		resolved := ""
		// only try these if bp is not absolute
		if !filepath.IsAbs(bp) {
			// a) next to the running client's executable
			if exe, err2 := os.Executable(); err2 == nil {
				candidate := filepath.Join(filepath.Dir(exe), bp)
				log.Printf("Debug: checking candidate next to exe: %q", candidate)
				if _, err3 := os.Stat(candidate); err3 == nil {
					log.Printf("Debug: candidate exists: %q", candidate)
					resolved = candidate
				} else {
					log.Printf("Debug: candidate missing: %q (%v)", candidate, err3)
				}
			} else {
				log.Printf("Debug: os.Executable() failed: %v", err2)
			}
			// b) ./bin/client/<bp> (common distributed bundle)
			if resolved == "" {
				candidate2 := filepath.Join("bin", "client", bp)
				log.Printf("Debug: checking candidate bin/client: %q", candidate2)
				if _, err4 := os.Stat(candidate2); err4 == nil {
					log.Printf("Debug: candidate exists: %q", candidate2)
					resolved = candidate2
				} else {
					log.Printf("Debug: candidate missing: %q (%v)", candidate2, err4)
				}
			}
			// c) try cwd + bp
			if resolved == "" {
				if cwd, err := os.Getwd(); err == nil {
					candidate3 := filepath.Join(cwd, bp)
					log.Printf("Debug: checking candidate cwd join: %q", candidate3)
					if _, err5 := os.Stat(candidate3); err5 == nil {
						log.Printf("Debug: candidate exists: %q", candidate3)
						resolved = candidate3
					} else {
						log.Printf("Debug: candidate missing: %q (%v)", candidate3, err5)
					}
				} else {
					log.Printf("Debug: os.Getwd() failed: %v", err)
				}
			}
		}
		// d) try to find on PATH
		if resolved == "" {
			log.Printf("Debug: trying exec.LookPath for %q", bp)
			if pth, err := exec.LookPath(bp); err == nil {
				log.Printf("Debug: LookPath found %q -> %q", bp, pth)
				resolved = pth
			} else {
				log.Printf("Debug: LookPath did not find %q: %v", bp, err)
			}
		}

		if resolved != "" {
			// convert to absolute path for exec
			if abs, err := filepath.Abs(resolved); err == nil {
				bp = abs
			} else {
				bp = resolved
			}
			cfg["bizhawk_path"] = bp
			log.Printf("resolved BizHawk path to %s", bp)
		}
	}

	// If the file still doesn't exist, try to install
	if _, err := os.Stat(bp); os.IsNotExist(err) {
		if err := EnsureBizHawkInstalled(httpClient, cfg); err != nil {
			return nil, fmt.Errorf("failed to install bizhawk: %w", err)
		}
	}
	if _, err := os.Stat(bp); err != nil {
		// final diagnostics: list surrounding directories and attempt a small scan
		log.Printf("LaunchBizHawk: final stat failed for %s: %v; doing diagnostic scan", bp, err)
		// list the parent directory
		if parent := filepath.Dir(bp); parent != "" {
			if ents, e := os.ReadDir(parent); e == nil {
				log.Printf("LaunchBizHawk: listing parent dir %s", parent)
				for _, en := range ents {
					info, _ := en.Info()
					log.Printf(" - %s (dir=%v mode=%v)", en.Name(), en.IsDir(), info.Mode())
				}
			} else {
				log.Printf("LaunchBizHawk: failed to read parent dir %s: %v", parent, e)
			}
			// quick scan common candidates under parent
			candidates := []string{"EmuHawk.exe", "DiscoHawk.exe", "EmuHawkMono.sh", "EmuHawk"}
			for _, c := range candidates {
				p := filepath.Join(parent, c)
				if fi, e := os.Stat(p); e == nil {
					log.Printf("LaunchBizHawk: found candidate during scan: %s (mode=%v)", p, fi.Mode())
				}
			}
		}
		return nil, fmt.Errorf("bizhawk not found at %s: %w", bp, err)
	}

	// Ensure bp is absolute for exec to avoid "The system cannot find the path specified"
	if !filepath.IsAbs(bp) {
		if abs, err := filepath.Abs(bp); err == nil {
			bp = abs
			cfg["bizhawk_path"] = bp
			log.Printf("LaunchBizHawk: converted bizhawk_path to absolute: %s", bp)
		} else {
			log.Printf("LaunchBizHawk: failed to convert bizhawk_path to abs: %v", err)
		}
	}

	// On Linux the launcher script expects args relative to the install dir,
	// and it changes working dir to the install dir. Emulate that by setting
	// Cmd.Dir to the install dir so relative paths work.
	args := []string{"--lua=../server.lua"}
	cmd := exec.CommandContext(ctx, bp, args...)
	// set working dir to the directory containing the executable
	cmd.Dir = filepath.Dir(bp)
	// ensure executable bit on non-windows
	if runtime.GOOS != "windows" {
		// try to chmod the script/executable to be executable; ignore errors
		_ = os.Chmod(bp, 0o755)
	}
	env := os.Environ()
	// player_name may be stored under either "player_name" (legacy) or "name" (canonical)
	if p, ok := cfg["player_name"]; ok && strings.TrimSpace(p) != "" {
		env = append(env, "BIZHAWK_PLAYER_NAME="+p)
	} else if p, ok := cfg["name"]; ok && strings.TrimSpace(p) != "" {
		env = append(env, "BIZHAWK_PLAYER_NAME="+p)
	}
	if rd, ok := cfg["rom_dir"]; ok {
		env = append(env, "BIZHAWK_ROM_DIR="+rd)
	}
	if sd, ok := cfg["save_dir"]; ok {
		env = append(env, "BIZHAWK_SAVE_DIR="+sd)
	}
	if ipc, ok := cfg["bizhawk_ipc_port"]; ok && ipc != "" {
		env = append(env, "BIZHAWK_IPC_PORT="+ipc)
	}
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	log.Printf("started BizHawk pid=%d", cmd.Process.Pid)
	return cmd, nil
}
