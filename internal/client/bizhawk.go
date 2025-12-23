package client

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/michael4d45/bizshuffle/internal/installer"
	"github.com/michael4d45/bizshuffle/internal/types"
)

// BizHawkController manages BizHawk installation, download and launching.
type BizHawkController struct {
	httpClient  *http.Client
	cfg         Config
	api         *API
	bipc        *BizhawkIPC
	wsClient    *WSClient
	initialized bool
}

// NewBizHawkController creates a new controller with provided API, http client and config.
func NewBizHawkController(api *API, httpClient *http.Client, cfg Config, bipc *BizhawkIPC, ws *WSClient) *BizHawkController {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &BizHawkController{httpClient: httpClient, cfg: cfg, api: api, bipc: bipc, wsClient: ws}
}

// VerifyBizHawkPath verifies that BizHawk is available at the configured path.
// Returns an error if bizhawk_path is not set or the file doesn't exist.
func (c *BizHawkController) VerifyBizHawkPath() error {
	bp := c.cfg["bizhawk_path"]
	if strings.TrimSpace(bp) == "" {
		return fmt.Errorf("bizhawk_path not configured: please run the installer or set bizhawk_path in config.json")
	}
	if _, err := os.Stat(bp); os.IsNotExist(err) {
		return fmt.Errorf("bizhawk not found at %s: please run the installer to install BizHawk", bp)
	}
	return nil
}

// LaunchBizHawk starts the BizHawk executable with environment variables and returns the *exec.Cmd.
func (c *BizHawkController) LaunchBizHawk(ctx context.Context) (*exec.Cmd, error) {
	bp := c.cfg["bizhawk_path"]
	if strings.TrimSpace(bp) == "" {
		return nil, fmt.Errorf("bizhawk_path not configured")
	}
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
			// b) try cwd + bp
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
		// c) try to find on PATH
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
			c.cfg["bizhawk_path"] = bp
			log.Printf("resolved BizHawk path to %s", bp)
		}
	}

	// Final verification that BizHawk exists at resolved path
	if _, err := os.Stat(bp); os.IsNotExist(err) {
		// Provide helpful diagnostics
		log.Printf("LaunchBizHawk: BizHawk not found at %s", bp)
		if parent := filepath.Dir(bp); parent != "" {
			if ents, e := os.ReadDir(parent); e == nil {
				log.Printf("LaunchBizHawk: listing parent dir %s", parent)
				for _, en := range ents {
					info, _ := en.Info()
					log.Printf(" - %s (dir=%v mode=%v)", en.Name(), en.IsDir(), info.Mode())
				}
			}
		}
		return nil, fmt.Errorf("bizhawk not found at %s: please run the installer to install BizHawk", bp)
	}

	// Ensure bp is absolute for exec to avoid "The system cannot find the path specified"
	if !filepath.IsAbs(bp) {
		if abs, err := filepath.Abs(bp); err == nil {
			bp = abs
			c.cfg["bizhawk_path"] = bp
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
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	log.Printf("started BizHawk pid=%d", cmd.Process.Pid)
	return cmd, nil
}

// LaunchAndManage starts BizHawk and takes ownership of lifecycle management:
// - launches the process (using LaunchBizHawk)
// - monitors the process and calls origCancel when it exits
// - listens for incoming signals on sigs and attempts graceful termination
// The function blocks until the context is cancelled or a shutdown signal is
// received and the process has been terminated. It returns any launch error.
func (c *BizHawkController) LaunchAndManage(ctx context.Context, origCancel func()) error {
	log.Printf("LaunchAndManage: starting BizHawk process and managing lifecycle")
	if !c.initialized {
		log.Printf("BizHawkController not initialized; waiting to launch BizHawk\n")

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			<-ticker.C
			if c.initialized {
				break
			}
		}
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	var bhCmd *exec.Cmd
	var bhMu sync.Mutex

	log.Printf("Debug: configured bizhawk_path=%q", c.cfg["bizhawk_path"])
	cmd, err := c.LaunchBizHawk(ctx)
	if err != nil {
		// if launch failed, cancel higher-level contexts
		if origCancel != nil {
			origCancel()
		}
		return fmt.Errorf("StartBizHawk failed: %w", err)
	}

	// Notify IPC that BizHawk has been launched
	c.bipc.SetBizhawkLaunched(true)
	bhMu.Lock()
	bhCmd = cmd
	bhMu.Unlock()

	if bhCmd != nil {
		log.Printf("monitoring BizHawk pid=%d", bhCmd.Process.Pid)
		MonitorProcess(bhCmd, func(err error) {
			log.Printf("MonitorProcess: BizHawk pid=%d exited with err=%v; cancelling client", bhCmd.Process.Pid, err)
			// Notify IPC that BizHawk has closed
			c.bipc.SetBizhawkLaunched(false)
			if origCancel != nil {
				origCancel()
			}
		})
	}

	// signal handling goroutine: listens for signals and attempts graceful shutdown
	go func() {
		select {
		case <-ctx.Done():
			return
		case s := <-sigs:
			log.Printf("signal: %v", s)
			log.Printf("terminating BizHawk due to signal: %v", s)
			TerminateProcess(&bhCmd, &bhMu, 3*time.Second)
			// Notify IPC that BizHawk has closed
			c.bipc.SetBizhawkLaunched(false)
			log.Printf("signal handler: calling origCancel() after TerminateProcess")
			if origCancel != nil {
				origCancel()
			}
		}
	}()

	// Wait for either context cancellation or a signal (the caller's signal
	// channel may also be drained elsewhere; we still check ctx.Done()).
	select {
	case <-ctx.Done():
		log.Printf("shutdown: context cancelled; terminating BizHawk and exiting")
	case s := <-sigs:
		log.Printf("received shutdown signal: %v; terminating BizHawk and exiting", s)
	}

	// Notify IPC that BizHawk has closed
	c.bipc.SetBizhawkLaunched(false)

	bhMu.Lock()
	if bhCmd != nil && bhCmd.Process != nil {
		if err := bhCmd.Process.Kill(); err != nil {
			log.Printf("failed to kill BizHawk process: %v", err)
		}
	}
	bhMu.Unlock()

	return nil
}

// TerminateProcess attempts to gracefully stop the given process and falls
// back to killing it after the provided grace duration. It accepts a pointer
// to the command and a mutex to coordinate access with callers that also use
// the same mutex (the pattern used in run.go). This preserves the original
// locking behavior while centralizing platform differences.
//
// On Windows the function will call Process.Kill() immediately because
// POSIX signals are not supported there in the same way. On other OSes it
// sends SIGTERM and schedules a forced kill after the grace period if the
// process hasn't exited.
func TerminateProcess(cmdPtr **exec.Cmd, mu *sync.Mutex, grace time.Duration) {
	if cmdPtr == nil || mu == nil {
		return
	}
	mu.Lock()
	cmd := *cmdPtr
	if cmd == nil || cmd.Process == nil {
		mu.Unlock()
		return
	}
	pid := cmd.Process.Pid

	// Windows: kill immediately
	if runtime.GOOS == "windows" {
		log.Printf("killing BizHawk pid=%d (windows)", pid)
		_ = cmd.Process.Kill()
		mu.Unlock()
		return
	}

	// POSIX: send SIGTERM and schedule a force kill after grace
	log.Printf("sending SIGTERM to BizHawk pid=%d", pid)
	_ = cmd.Process.Signal(syscall.SIGTERM)
	mu.Unlock()

	if grace <= 0 {
		return
	}

	time.AfterFunc(grace, func() {
		mu.Lock()
		defer mu.Unlock()
		if *cmdPtr != nil && (*cmdPtr).ProcessState == nil {
			log.Printf("killing BizHawk pid=%d after grace", pid)
			_ = (*cmdPtr).Process.Kill()
		}
	})
}

// GetInstalledVersion returns the currently installed version of BizHawk.
func (c *BizHawkController) GetInstalledVersion() string {
	return c.cfg["bizhawk_version"]
}

// GetLatestVersion fetches the latest available BizHawk version from GitHub.
func (c *BizHawkController) GetLatestVersion() (string, error) {
	rel, err := installer.GetBizHawkLatestRelease()
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(rel.TagName, "v"), nil
}

// UpdateBizHawk downloads and updates BizHawk to the latest version.
func (c *BizHawkController) UpdateBizHawk(progress func(string)) error {
	if progress == nil {
		progress = func(string) {}
	}

	progress("Checking for latest BizHawk version...")
	rel, err := installer.GetBizHawkLatestRelease()
	if err != nil {
		return fmt.Errorf("failed to get latest release: %w", err)
	}

	tagName := strings.TrimPrefix(rel.TagName, "v")
	progress(fmt.Sprintf("Latest version is %s", tagName))

	// Find the appropriate asset
	platformSuffix := installer.GetBizHawkPlatformSuffix()
	assetName := fmt.Sprintf("BizHawk-%s-%s.zip", tagName, platformSuffix)
	asset := rel.FindAssetByName(assetName)
	if asset == nil {
		// Try alternative naming pattern
		assetName = fmt.Sprintf("BizHawk-%s-%s.zip", rel.TagName, platformSuffix)
		asset = rel.FindAssetByName(assetName)
	}

	if asset == nil {
		// Fallback: search for any zip with platform suffix
		for _, a := range rel.Assets {
			if strings.Contains(a.Name, platformSuffix) && strings.HasSuffix(a.Name, ".zip") {
				asset = &a
				break
			}
		}
	}

	if asset == nil {
		return fmt.Errorf("could not find BizHawk asset for platform %s", platformSuffix)
	}

	bp := c.cfg["bizhawk_path"]
	if bp == "" {
		return fmt.Errorf("bizhawk_path not set")
	}
	bizhawkDir := filepath.Dir(bp)

	// Backup config.ini and Firmware if they exist
	progress("Backing up configuration and firmware...")
	configPath := filepath.Join(bizhawkDir, "config.ini")
	firmwarePath := filepath.Join(bizhawkDir, "Firmware")
	tempDir, err := os.MkdirTemp("", "bizhawk-backup")
	if err != nil {
		return fmt.Errorf("failed to create temp dir for backup: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	configBackup := filepath.Join(tempDir, "config.ini")
	hasConfig := false
	if _, err := os.Stat(configPath); err == nil {
		if err := copyFile(configPath, configBackup); err != nil {
			return fmt.Errorf("failed to backup config.ini: %w", err)
		}
		hasConfig = true
	}

	firmwareBackup := filepath.Join(tempDir, "Firmware")
	hasFirmware := false
	if _, err := os.Stat(firmwarePath); err == nil {
		if err := copyDir(firmwarePath, firmwareBackup); err != nil {
			return fmt.Errorf("failed to backup Firmware: %w", err)
		}
		hasFirmware = true
	}

	progress(fmt.Sprintf("Downloading BizHawk %s...", tagName))
	bhInstaller := installer.NewBizHawkInstaller()
	if err := bhInstaller.InstallBizHawk(asset.DownloadURL, bizhawkDir, progress); err != nil {
		return fmt.Errorf("failed to install BizHawk: %w", err)
	}

	// Restore backups
	if hasConfig {
		progress("Restoring config.ini...")
		if err := copyFile(configBackup, configPath); err != nil {
			log.Printf("Warning: failed to restore config.ini: %v", err)
		}
	}
	if hasFirmware {
		progress("Restoring Firmware...")
		if err := copyDir(firmwareBackup, firmwarePath); err != nil {
			log.Printf("Warning: failed to restore Firmware: %v", err)
		}
	}

	c.cfg["bizhawk_version"] = tagName
	if err := c.cfg.Save(); err != nil {
		log.Printf("Warning: failed to save config after update: %v", err)
	}

	progress("BizHawk updated successfully")
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		return copyFile(path, targetPath)
	})
}

func (c *BizHawkController) StartIPCGoroutine(ctx context.Context) {
	// Use API.FetchServerState to query the server state for this client/player.
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Printf("bizhawk ipc: StartIPCGoroutine context cancelled, exiting")
				return
			case line, ok := <-c.bipc.Incoming():
				if !ok {
					// Channel closed
					log.Printf("bizhawk ipc: incoming channel closed or handler goroutine exiting; marking ipcReady=false")
					if c.bipc != nil {
						c.bipc.SetReady(false)
						// Notify server that BizHawk is no longer ready
						if err := c.wsClient.SendBizhawkReadinessUpdate(false); err != nil {
							log.Printf("ipc handler: failed to send BizHawk readiness update: %v", err)
						}
					}
					return
				}

				if line == MsgDisconnected {
					log.Printf("bizhawk ipc: disconnected detected from readLoop (ipc handler); marking ipcReady=false and continuing")
					if c.bipc != nil {
						c.bipc.SetReady(false)
						// Notify server that BizHawk is no longer ready
						if err := c.wsClient.SendBizhawkReadinessUpdate(false); err != nil {
							log.Printf("ipc handler: failed to send BizHawk readiness update: %v", err)
						}
					}
					// don't cancel the main context here; allow reconnect logic to run
					continue
				}
				log.Printf("lua incoming: %s", line)
				if strings.HasPrefix(line, msgHELLO) {
					log.Printf("ipc handler: received HELLO from lua")
					c.bipc.SetReady(true)

					// Notify server that BizHawk is now ready
					if err := c.wsClient.SendBizhawkReadinessUpdate(true); err != nil {
						log.Printf("ipc handler: failed to send BizHawk readiness update: %v", err)
					}

					running := c.bipc.running
					game := c.bipc.game
					instanceID := c.bipc.instanceID
					var err error
					if game != "" && running {
						log.Printf("ipc handler: BizHawk restarted, re-sending START for game %q instance %q", game, instanceID)
					} else {
						running, game, instanceID, err = c.api.FetchServerState(c.cfg["name"])
						if err != nil {
							log.Printf("ipc handler: FetchServerState failed: %v; defaulting running=true, empty game", err)
							running = true
							game = ""
						}
					}
					if game == "" {
						log.Printf("ipc handler: no current game for player from server state; sending empty game")
					}
					ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
					if err := c.api.EnsureSaveState(instanceID); err != nil {
						log.Printf("ipc handler: EnsureSaveState failed: %v", err)
					}
					if err := c.bipc.SendPause(ctx2); err != nil {
						log.Printf("ipc handler: failed to send PAUSE command to lua: %v", err)
					}
					if err := c.bipc.SendSwap(ctx2, game, instanceID); err != nil {
						log.Printf("ipc handler: failed to send SWAP command to lua: %v", err)
					}
					if running {
						if err := c.bipc.SendResume(ctx2); err != nil {
							log.Printf("ipc handler: failed to send RESUME command to lua: %v", err)
						}
					}
					cancel2()

				}
				// example: lua incoming: CMD|message|message=Read Door: Legend of Zelda, The - A Link to the Past (USA).zip room value changed: nil -> 514 (room id)
				if strings.HasPrefix(line, msgCMD) {
					// Parse and log Lua command messages for now.
					if cmd, err := types.ParseLuaCommand(line); err != nil {
						log.Printf("ipc handler: failed to parse CMD line: %v", err)
					} else {
						log.Printf("ipc handler: parsed CMD: kind=%q fields=%v", cmd.Kind, cmd.Fields)
						if err := c.wsClient.Send(
							types.Command{
								Cmd:     types.CmdTypeLua,
								Payload: cmd,
							}); err != nil {
							log.Printf("ipc handler: failed to send CMD: %v", err)
						}
					}
				}
			}
		}
	}()
}

// EnsureBizhawkFiles downloads and extracts BizhawkFiles.zip if config.ini doesn't exist
// in the BizHawk directory. Returns nil if files already exist or were successfully downloaded.
func (c *BizHawkController) EnsureBizhawkFiles() error {
	if c.api == nil {
		return fmt.Errorf("API not available, cannot download BizhawkFiles")
	}

	bp := c.cfg["bizhawk_path"]
	if strings.TrimSpace(bp) == "" {
		return fmt.Errorf("bizhawk_path not configured")
	}

	// Resolve BizHawk path similar to LaunchBizHawk
	// First, make sure it's absolute
	if !filepath.IsAbs(bp) {
		if abs, err := filepath.Abs(bp); err == nil {
			bp = abs
		}
	}

	// Get BizHawk directory (directory containing the executable)
	bizhawkDir := filepath.Dir(bp)
	if !filepath.IsAbs(bizhawkDir) {
		var err error
		bizhawkDir, err = filepath.Abs(bizhawkDir)
		if err != nil {
			return fmt.Errorf("failed to resolve BizHawk directory: %w", err)
		}
	}

	// Check if config.ini exists
	configIniPath := filepath.Join(bizhawkDir, "config.ini")
	if _, err := os.Stat(configIniPath); err == nil {
		log.Printf("BizhawkFiles already present (config.ini exists)")
		return nil
	}

	log.Printf("config.ini not found in %s, downloading BizhawkFiles.zip...", bizhawkDir)

	// Download BizhawkFiles.zip
	bizFilesURL := c.api.BizhawkFilesURL()
	tempZip := filepath.Join(os.TempDir(), "BizhawkFiles.zip")
	defer func() { _ = os.Remove(tempZip) }()

	// Download the file
	log.Printf("Downloading BizhawkFiles.zip from %s...", bizFilesURL)
	resp, err := c.httpClient.Get(bizFilesURL)
	if err != nil {
		return fmt.Errorf("failed to download BizhawkFiles.zip: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status %s", resp.Status)
	}

	// Save to temporary file
	out, err := os.Create(tempZip)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	_, err = io.Copy(out, resp.Body)
	if cerr := out.Close(); cerr != nil {
		if err == nil {
			err = cerr
		}
	}
	if err != nil {
		return fmt.Errorf("failed to save BizhawkFiles.zip: %w", err)
	}

	// Extract the zip file
	log.Printf("Extracting BizhawkFiles.zip to %s...", bizhawkDir)
	if err := c.extractZip(tempZip, bizhawkDir); err != nil {
		return fmt.Errorf("failed to extract BizhawkFiles.zip: %w", err)
	}

	// Verify config.ini was extracted
	if _, err := os.Stat(configIniPath); os.IsNotExist(err) {
		return fmt.Errorf("config.ini not found after extraction, extraction may have failed")
	}

	log.Printf("BizhawkFiles.zip downloaded and extracted successfully")
	return nil
}

// extractZip extracts a zip file to the destination directory
func (c *BizHawkController) extractZip(zipPath, destDir string) error {
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
			if err == nil {
				err = cerr
			}
		}
		if cerr := rc.Close(); cerr != nil {
			if err == nil {
				err = cerr
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}
