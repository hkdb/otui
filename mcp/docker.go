package mcp

import (
	"strings"
)

// buildDockerCommand returns the command for a docker plugin. A configured
// startup command (plugins.toml __command__ or the plugin's Command) is used
// as is. Otherwise the image (Package) runs as a stdio MCP server:
//
//	docker run -i --rm [-e KEY...] [options] <image> [container args]
//
// Env vars are passed by name only (-e KEY): their values reach the docker
// CLI through the plugin process environment, so they don't show up in the
// process list. Args are docker run options; tokens after "--" are passed to
// the container. Returns "" if there's no image.
func buildDockerCommand(plugin *Plugin, pluginConfig map[string]string) (string, []string) {
	for _, command := range []string{pluginConfig["__command__"], plugin.Command} {
		if parts := strings.Fields(command); len(parts) > 0 {
			return parts[0], parts[1:]
		}
	}

	image := strings.TrimSpace(plugin.Package)
	if image == "" {
		return "", nil
	}

	args := []string{"run", "-i", "--rm"}
	for _, key := range EnvironmentKeys(plugin.Environment) {
		if pluginConfig[key] != "" {
			args = append(args, "-e", key)
		}
	}

	options := SubstituteArgs(plugin.Args, pluginConfig)
	var containerArgs []string
	for i, arg := range options {
		if arg == "--" {
			options, containerArgs = options[:i], options[i+1:]
			break
		}
	}

	args = append(args, options...)
	args = append(args, image)
	args = append(args, containerArgs...)
	return "docker", args
}
