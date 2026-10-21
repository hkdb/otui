//go:build !windows

package mcp

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

// setProcessGroup starts cmd in its own process group so the whole tree
// (e.g. npx/uvx/docker and the server they start) can be stopped together
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateProcessGroup stops cmd's process group: SIGTERM first (docker run
// forwards it to the container), then SIGKILL if anything is left after 1s
func terminateProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid := -cmd.Process.Pid

	if err := syscall.Kill(pgid, syscall.SIGTERM); errors.Is(err, syscall.ESRCH) {
		return nil
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pgid, 0); errors.Is(err, syscall.ESRCH) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := syscall.Kill(pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
