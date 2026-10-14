//go:build windows

package config

import "os"

// processAlive reports whether a process with this PID is running
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// On Windows FindProcess opens the process and fails if it doesn't exist
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	p.Release()
	return true
}
