package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

// DownloadAndExtractZip downloads a zip file and extracts it into destDir.
func DownloadAndExtractZip(client *http.Client, url, zipPath, destDir string) error {
	if err := DownloadFile(client, url, zipPath); err != nil {
		return err
	}
	defer os.Remove(zipPath)
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
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
			outFile.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
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
	installDir := strings.TrimSuffix(zipFile, filepath.Ext(zipFile))
	// expected exe path inside installDir
	expected := cfg["bizhawk_path"]
	if expected == "" {
		expected = filepath.Join(installDir, "EmuHawk.exe")
		cfg["bizhawk_path"] = expected
	}
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		log.Printf("BizHawk not found at %s, downloading...", expected)
		if err := DownloadAndExtractZip(httpClient, downloadURL, zipFile, installDir); err != nil {
			return fmt.Errorf("failed to download/extract BizHawk: %w", err)
		}
		// optional: look for additional files from server
		if server, ok := cfg["server"]; ok && server != "" {
			bizFilesURL := strings.TrimSuffix(server, "/") + "/api/BizhawkFiles.zip"
			if err := DownloadAndExtractZip(httpClient, bizFilesURL, "BizhawkFiles.zip", installDir); err != nil {
				log.Printf("warning: failed to download BizhawkFiles.zip: %v", err)
			}
		}
		log.Printf("BizHawk installed into %s", installDir)
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
	// If the file doesn't exist, try to install
	if _, err := os.Stat(bp); os.IsNotExist(err) {
		if err := EnsureBizHawkInstalled(httpClient, cfg); err != nil {
			return nil, fmt.Errorf("failed to install bizhawk: %w", err)
		}
	}
	if _, err := os.Stat(bp); err != nil {
		return nil, fmt.Errorf("bizhawk not found at %s: %w", bp, err)
	}

	args := []string{}
	args = append(args, "--lua=server.lua")
	cmd := exec.CommandContext(ctx, bp, args...)
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
