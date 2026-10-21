//go:build !windows

package config

import (
	"errors"
	"os"
	"syscall"
)

// processAlive reports whether a process with this PID is running
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid) // Always succeeds on Unix
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	// EPERM: the process exists but belongs to another user
	return err == nil || errors.Is(err, syscall.EPERM)
}
