//go:build !windows

package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// testMCPModeEnv makes the test binary act as a stdio MCP server (see
// TestMain), so plugin processes can be tested without external tools
const testMCPModeEnv = "OTUI_TEST_MCP_MODE"

// testPIDFileEnv makes the test MCP server write its PID to a file
const testPIDFileEnv = "OTUI_TEST_PID_FILE"

func TestMain(m *testing.M) {
	if mode := os.Getenv(testMCPModeEnv); mode != "" {
		runTestMCPServer(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func newTestMCPServer() *server.MCPServer {
	s := server.NewMCPServer("test", "1.0.0")
	s.AddTool(mcptypes.NewTool("echo"), func(ctx context.Context, req mcptypes.CallToolRequest) (*mcptypes.CallToolResult, error) {
		return mcptypes.NewToolResultText("pong"), nil
	})
	return s
}

func runTestMCPServer(mode string) {
	if f := os.Getenv(testPIDFileEnv); f != "" {
		_ = os.WriteFile(f, []byte(strconv.Itoa(os.Getpid())), 0o600)
	}

	switch mode {
	case "chatty":
		// More than a pipe buffer of logs before serving
		line := strings.Repeat("x", 1023) + "\n"
		for i := 0; i < 1024; i++ {
			os.Stderr.WriteString(line)
		}
	case "hang":
		// Never answer initialize
		time.Sleep(time.Minute)
		return
	case "sleep":
		time.Sleep(time.Minute)
		return
	case "wrapper":
		startTestGrandchild()
	}
	_ = server.ServeStdio(newTestMCPServer())
}

// testPluginConfig returns a local plugin config that runs the test binary
// as an MCP server in the given mode
func testPluginConfig(id, mode string, extraEnv map[string]string) PluginConfig {
	env := map[string]string{testMCPModeEnv: mode}
	for k, v := range extraEnv {
		env[k] = v
	}
	return PluginConfig{ID: id, EntryPoint: os.Args[0], Env: env}
}

func newTestProcessManager() *ProcessManager {
	return NewProcessManager("", nil)
}

func TestStartPluginChattyStderr(t *testing.T) {
	pm := newTestProcessManager()
	defer pm.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pm.StartPlugin(ctx, testPluginConfig("chatty", "chatty", nil)); err != nil {
		t.Fatalf("StartPlugin: %v", err)
	}

	c, err := pm.GetClient("chatty")
	if err != nil {
		t.Fatal(err)
	}
	req := mcptypes.CallToolRequest{}
	req.Params.Name = "echo"
	if _, err := c.CallTool(ctx, req); err != nil {
		t.Fatalf("CallTool: %v", err)
	}
}

func TestStartPluginFailedInitializeCleansUp(t *testing.T) {
	pm := newTestProcessManager()
	defer pm.Shutdown(context.Background())

	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pm.StartPlugin(ctx, testPluginConfig("hang", "hang", map[string]string{testPIDFileEnv: pidFile})); err == nil {
		t.Fatal("expected StartPlugin to fail")
	}

	pm.mu.RLock()
	n := len(pm.processes)
	pm.mu.RUnlock()
	if n != 0 {
		t.Fatalf("%d processes registered after failed start, want 0", n)
	}
	waitProcessGone(t, readPIDFile(t, pidFile))
}

func TestStartPluginConcurrent(t *testing.T) {
	pm := newTestProcessManager()
	defer pm.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = pm.StartPlugin(ctx, testPluginConfig("dup", "serve", nil))
		}(i)
	}
	wg.Wait()

	failed := 0
	for _, err := range errs {
		if err != nil {
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("want exactly one start to fail, got errors %v", errs)
	}
}

func TestStartRemoteSSEPluginOutlivesStartContext(t *testing.T) {
	ts := server.NewTestServer(newTestMCPServer())
	defer ts.Close()

	pm := newTestProcessManager()
	defer pm.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := pm.StartPlugin(ctx, PluginConfig{ID: "remote", ServerURL: ts.URL + "/sse", Transport: "sse", AuthType: "none"})
	cancel() // Callers cancel their ctx as soon as the start returns
	if err != nil {
		t.Fatalf("StartPlugin: %v", err)
	}

	c, err := pm.GetClient("remote")
	if err != nil {
		t.Fatal(err)
	}
	callCtx, callCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer callCancel()
	req := mcptypes.CallToolRequest{}
	req.Params.Name = "echo"
	if _, err := c.CallTool(callCtx, req); err != nil {
		t.Fatalf("CallTool after start ctx was cancelled: %v", err)
	}
}

func readPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no PID written to %s", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
