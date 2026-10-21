//go:build windows

package mcp

import "os/exec"

func setProcessGroup(cmd *exec.Cmd) {}

// terminateProcessGroup kills the plugin process (child processes aren't
// tracked on Windows)
func terminateProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
