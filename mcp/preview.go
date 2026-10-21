package mcp

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"otui/storage"
)

// PluginFolderPlaceholder stands for a plugin's install directory in a
// command preview before it's installed
const PluginFolderPlaceholder = "<plugin-folder>"

// secretLabelRe matches config fields whose values are masked in previews
var secretLabelRe = regexp.MustCompile(`(?i)key|token|secret|password|passwd|credential|auth`)

// CommandPreview describes what a plugin will run, so it's visible before
// it's installed or started: the install command (empty if nothing is
// installed), the start command (or the URL a remote plugin connects to),
// and warnings about risky docker options. Secret values are masked.
// installPath is the plugin's directory, or "" before it's installed.
func CommandPreview(plugin *Plugin, pluginConfig map[string]string, installPath string) (install, run string, warnings []string) {
	switch plugin.InstallType {
	case "npm":
		install = "npm install -- " + plugin.Package
	case "pip":
		install = "pip install -- " + plugin.Package + "  (in a new virtual environment)"
	case "go":
		install = "go install -- " + plugin.Package
	case "docker":
		if plugin.Package != "" {
			install = "docker pull -- " + plugin.Package
		}
	case "remote":
		url := plugin.ServerURL
		if override := pluginConfig["server_url"]; override != "" {
			url = override
		}
		if sendsCredentials(plugin, pluginConfig) && insecureRemote(url) {
			warnings = append(warnings, "sends its credentials unencrypted (http://)")
		}
		return "", "Connects to: " + url, warnings
	}

	if installPath == "" {
		installPath = PluginFolderPlaceholder
	}
	command, args := pluginCommand(plugin, &storage.InstalledPlugin{InstallPath: installPath}, maskSecrets(plugin, pluginConfig))
	if command == "" {
		return install, "", nil
	}

	run = joinCommand(command, args)
	if plugin.InstallType == "docker" || plugin.InstallType == "manual" {
		warnings = dockerWarnings(command, args)
	}
	return install, run, warnings
}

// sendsCredentials reports whether a remote plugin sends auth headers or
// uses OAuth
func sendsCredentials(plugin *Plugin, pluginConfig map[string]string) bool {
	if plugin.AuthType == "oauth" {
		return true
	}
	for key, value := range pluginConfig {
		if value != "" && !isInternalConfigKey(key) {
			return true
		}
	}
	return false
}

// insecureRemote reports whether url is plain http:// to a host other than
// this machine
func insecureRemote(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}

// maskSecrets returns the config with secret values (by field name or arg
// label) replaced by ****
func maskSecrets(plugin *Plugin, pluginConfig map[string]string) map[string]string {
	masked := make(map[string]string, len(pluginConfig))
	for key, value := range pluginConfig {
		if isSecretValue(value) && secretLabelRe.MatchString(key) {
			value = "****"
		}
		masked[key] = value
	}
	for i, def := range parseArgsWithDefaults(plugin.Args) {
		key := "ARG_" + strconv.Itoa(i)
		if isSecretValue(masked[key]) && secretLabelRe.MatchString(def.Label) {
			masked[key] = "****"
		}
	}
	return masked
}

// isSecretValue reports whether a value needs masking: placeholders like
// "<API Key>" (nothing filled in yet) are shown as they are
func isSecretValue(value string) bool {
	return value != "" && !(strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">"))
}

// joinCommand formats a command line, quoting arguments with spaces
// (placeholders like <API Key> are shown as they are)
func joinCommand(command string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	for _, part := range append([]string{command}, args...) {
		if part == "" || strings.ContainsAny(part, " \t\"'") && isSecretValue(part) {
			part = strconv.Quote(part)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " ")
}

// dockerWarnings flags docker run options that give a container access to
// the host
func dockerWarnings(command string, args []string) []string {
	if command != "docker" && !strings.HasSuffix(command, "/docker") && command != "podman" {
		return nil
	}

	var warnings []string
	for i, arg := range args {
		next := ""
		if i+1 < len(args) {
			next = args[i+1]
		}
		// "--opt=value" or "--opt value"
		name, value, hasValue := strings.Cut(arg, "=")
		option := arg
		if !hasValue {
			value = next
			option = strings.TrimSpace(arg + " " + next)
		}

		switch name {
		case "--privileged":
			warnings = append(warnings, "gives the container full privileges (--privileged)")
		case "-v", "--volume", "--mount":
			warnings = append(warnings, "gives the container access to your files ("+option+")")
		case "--pid", "--ipc", "--uts", "--userns":
			if value == "host" {
				warnings = append(warnings, "shares the host's "+strings.TrimPrefix(name, "--")+" namespace ("+option+")")
			}
		case "--network", "--net":
			if value == "host" {
				warnings = append(warnings, "uses the host's network ("+option+")")
			}
		case "--cap-add", "--device":
			warnings = append(warnings, "adds host capabilities or devices ("+option+")")
		}
	}
	return warnings
}
