package client

import (
	"log"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
)

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
