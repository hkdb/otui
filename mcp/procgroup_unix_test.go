//go:build !windows

package mcp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// testGrandchildPIDFileEnv makes the "wrapper" test server start a
// grandchild that writes its PID to this file, like npx/uvx starting the
// real server
const testGrandchildPIDFileEnv = "OTUI_TEST_GRANDCHILD_PID_FILE"

func startTestGrandchild() {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(),
		testMCPModeEnv+"=sleep",
		testPIDFileEnv+"="+os.Getenv(testGrandchildPIDFileEnv))
	_ = cmd.Start()
}

// processGone reports whether pid no longer exists (or is a zombie waiting
// to be reaped)
func processGone(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return true
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// The state follows the parenthesised command name
	if i := strings.LastIndexByte(string(stat), ')'); i >= 0 && i+2 < len(stat) {
		return stat[i+2] == 'Z'
	}
	return false
}

func waitProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !processGone(pid) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("process %d still running", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStopPluginKillsGrandchildren(t *testing.T) {
	pm := newTestProcessManager()
	defer pm.Shutdown(context.Background())

	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := testPluginConfig("wrapper", "wrapper", map[string]string{testGrandchildPIDFileEnv: pidFile})
	if err := pm.StartPlugin(ctx, cfg); err != nil {
		t.Fatalf("StartPlugin: %v", err)
	}
	grandchild := readPIDFile(t, pidFile)

	if err := pm.StopPlugin(context.Background(), "wrapper"); err != nil {
		t.Fatalf("StopPlugin: %v", err)
	}
	waitProcessGone(t, grandchild)
}
