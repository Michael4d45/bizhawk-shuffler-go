package client

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/exec"
)

// StartBizHawk is a thin wrapper around LaunchBizHawk that returns the
// started command. It exists to centralize future logic (retries, metrics
// etc.) and to provide a clear place for dependency injection in tests.
func StartBizHawk(ctx context.Context, cfg Config, httpClient *http.Client) (*exec.Cmd, error) {
	// LaunchBizHawk expects a map[string]string
	cfgMap := map[string]string{}
	for k, v := range cfg {
		cfgMap[k] = v
	}
	cmd, err := LaunchBizHawk(ctx, cfgMap, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to launch BizHawk: %w", err)
	}
	return cmd, nil
}

// MonitorProcess starts a goroutine that waits for the process to exit and
// calls onExit(err). onExit will be called regardless of whether the process
// exited successfully or with an error.
func MonitorProcess(cmd *exec.Cmd, onExit func(error)) {
	if cmd == nil {
		go onExit(fmt.Errorf("nil cmd"))
		return
	}
	go func() {
		err := cmd.Wait()
		if err != nil {
			log.Printf("BizHawk exited with error: %v", err)
		} else {
			log.Printf("BizHawk exited")
		}
		onExit(err)
	}()
}
