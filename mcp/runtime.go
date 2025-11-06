package mcp

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type Runtime struct {
	Name      string
	Installed bool
	Version   string
	Path      string
	Error     string
}

type RuntimeChecker struct {
	runtimes map[string]*Runtime
}

func NewRuntimeChecker() *RuntimeChecker {
	return &RuntimeChecker{
		runtimes: make(map[string]*Runtime),
	}
}

func (rc *RuntimeChecker) DetectAll() {
	rc.detectNodeJS()
	rc.detectNPX()
	rc.detectPython()
	rc.detectGo()
}

func (rc *RuntimeChecker) CheckRuntime(name string) (*Runtime, error) {
	runtime, ok := rc.runtimes[name]
	if !ok {
		switch name {
		case "node":
			rc.detectNodeJS()
		case "npx":
			rc.detectNPX()
		case "python":
			rc.detectPython()
		case "go":
			rc.detectGo()
		default:
			return nil, fmt.Errorf("unknown runtime: %s", name)
		}
		runtime = rc.runtimes[name]
	}

	if !runtime.Installed {
		if runtime.Error != "" {
			return nil, fmt.Errorf("%s", runtime.Error)
		}
		return nil, fmt.Errorf("%s not found", name)
	}

	return runtime, nil
}

func (rc *RuntimeChecker) CheckVersion(name, minVersion string) error {
	runtime, err := rc.CheckRuntime(name)
	if err != nil {
		return err
	}

	if !meetsMinVersion(runtime.Version, minVersion) {
		return fmt.Errorf("%s version %s (requires >= %s)", name, runtime.Version, minVersion)
	}

	return nil
}

func (rc *RuntimeChecker) GetAll() map[string]*Runtime {
	rc.DetectAll()
	return rc.runtimes
}

func (rc *RuntimeChecker) detectNodeJS() {
	runtime := &Runtime{Name: "node"}

	path, err := exec.LookPath("node")
	if err != nil {
		runtime.Error = "Node.js not found"
		rc.runtimes["node"] = runtime
		return
	}

	cmd := exec.Command("node", "--version")
	output, err := cmd.Output()
	if err != nil {
		runtime.Error = "Failed to get Node.js version"
		rc.runtimes["node"] = runtime
		return
	}

	version := strings.TrimSpace(string(output))
	version = strings.TrimPrefix(version, "v")

	runtime.Installed = true
	runtime.Version = version
	runtime.Path = path
	rc.runtimes["node"] = runtime
}

func (rc *RuntimeChecker) detectNPX() {
	runtime := &Runtime{Name: "npx"}

	path, err := exec.LookPath("npx")
	if err != nil {
		runtime.Error = "npx not found"
		rc.runtimes["npx"] = runtime
		return
	}

	cmd := exec.Command("npx", "--version")
	output, err := cmd.Output()
	if err != nil {
		runtime.Error = "Failed to get npx version"
		rc.runtimes["npx"] = runtime
		return
	}

	version := strings.TrimSpace(string(output))

	runtime.Installed = true
	runtime.Version = version
	runtime.Path = path
	rc.runtimes["npx"] = runtime
}

func (rc *RuntimeChecker) detectPython() {
	runtime := &Runtime{Name: "python"}

	pythonCmd := ""
	for _, cmd := range []string{"python3", "python"} {
		if _, err := exec.LookPath(cmd); err == nil {
			pythonCmd = cmd
			break
		}
	}

	if pythonCmd == "" {
		runtime.Error = "Python not found"
		rc.runtimes["python"] = runtime
		return
	}

	path, _ := exec.LookPath(pythonCmd)
	runtime.Path = path

	versionCmd := exec.Command(pythonCmd, "--version")
	output, err := versionCmd.Output()
	if err != nil {
		runtime.Error = "Failed to get Python version"
		rc.runtimes["python"] = runtime
		return
	}

	version := strings.TrimSpace(string(output))
	version = strings.TrimPrefix(version, "Python ")
	runtime.Version = version

	venvCheck := exec.Command(pythonCmd, "-m", "venv", "--help")
	if err := venvCheck.Run(); err != nil {
		runtime.Installed = false
		runtime.Error = "Python venv module not available"
		rc.runtimes["python"] = runtime
		return
	}

	runtime.Installed = true
	rc.runtimes["python"] = runtime
}

func (rc *RuntimeChecker) detectGo() {
	runtime := &Runtime{Name: "go"}

	path, err := exec.LookPath("go")
	if err != nil {
		runtime.Error = "Go not found"
		rc.runtimes["go"] = runtime
		return
	}

	cmd := exec.Command("go", "version")
	output, err := cmd.Output()
	if err != nil {
		runtime.Error = "Failed to get Go version"
		rc.runtimes["go"] = runtime
		return
	}

	version := strings.TrimSpace(string(output))
	re := regexp.MustCompile(`go(\d+\.\d+(?:\.\d+)?)`)
	matches := re.FindStringSubmatch(version)
	if len(matches) > 1 {
		version = matches[1]
	}

	runtime.Installed = true
	runtime.Version = version
	runtime.Path = path
	rc.runtimes["go"] = runtime
}

func meetsMinVersion(current, minimum string) bool {
	currentParts := parseVersion(current)
	minimumParts := parseVersion(minimum)

	for i := 0; i < 3; i++ {
		if currentParts[i] > minimumParts[i] {
			return true
		}
		if currentParts[i] < minimumParts[i] {
			return false
		}
	}

	return true
}

func parseVersion(version string) [3]int {
	parts := strings.Split(version, ".")
	var result [3]int

	for i := 0; i < 3 && i < len(parts); i++ {
		num, _ := strconv.Atoi(parts[i])
		result[i] = num
	}

	return result
}
