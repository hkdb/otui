package mcp

import (
	"os"
	"runtime"
	"strings"
)

// Plugins don't inherit OTUI's whole environment (which can hold API keys
// and tokens): only the variables a program commonly needs to run, like the
// official MCP SDKs do. A plugin gets anything else through its config.

// inheritedEnvNames are passed to plugins when set
var inheritedEnvNames = []string{
	"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TERM", "LANG", "LANGUAGE", "TZ", "TMPDIR",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "no_proxy", "all_proxy",
	"DISPLAY", "WAYLAND_DISPLAY", "DBUS_SESSION_BUS_ADDRESS",
	"DOCKER_HOST", "DOCKER_CONFIG", "DOCKER_CONTEXT",
	"NVM_DIR", "SSL_CERT_FILE", "SSL_CERT_DIR",
}

// inheritedEnvPrefixes are passed to plugins (locale, XDG directories)
var inheritedEnvPrefixes = []string{"LC_", "XDG_"}

// windowsEnvNames are also passed on Windows (names are case-insensitive)
var windowsEnvNames = []string{
	"APPDATA", "LOCALAPPDATA", "HOMEDRIVE", "HOMEPATH", "USERPROFILE", "USERNAME",
	"SYSTEMROOT", "SYSTEMDRIVE", "WINDIR", "COMSPEC", "PATHEXT", "TEMP", "TMP",
	"PROGRAMFILES", "PROGRAMFILES(X86)", "PROGRAMDATA", "PROCESSOR_ARCHITECTURE",
}

// inheritedEnv returns the variables of OTUI's environment that plugins get
func inheritedEnv() []string {
	return filterEnv(os.Environ(), runtime.GOOS == "windows")
}

func filterEnv(environ []string, windows bool) []string {
	allowed := make(map[string]bool)
	for _, name := range inheritedEnvNames {
		allowed[envKey(name, windows)] = true
	}
	if windows {
		for _, name := range windowsEnvNames {
			allowed[envKey(name, true)] = true
		}
	}

	var env []string
	for _, kv := range environ {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		if allowed[envKey(name, windows)] || hasEnvPrefix(name, windows) {
			env = append(env, kv)
		}
	}
	return env
}

func envKey(name string, windows bool) string {
	if windows {
		return strings.ToUpper(name)
	}
	return name
}

func hasEnvPrefix(name string, windows bool) bool {
	for _, prefix := range inheritedEnvPrefixes {
		if strings.HasPrefix(envKey(name, windows), prefix) {
			return true
		}
	}
	return false
}

// isInternalConfigKey reports whether a plugin config key is an OTUI
// setting (startup command, argument values, server URL override) rather
// than one of the plugin's own variables
func isInternalConfigKey(key string) bool {
	return key == "__command__" || key == "server_url" || strings.HasPrefix(key, "ARG_")
}
