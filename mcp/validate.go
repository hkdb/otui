package mcp

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// Plugin data comes from the (unsigned) registry or the custom plugin form.
// It ends up in file paths and command lines, so it's validated first.

var (
	pluginIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

	// Go package path with optional @version (go install accepts nothing else)
	goPackageRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~/+-]*(@[A-Za-z0-9._~+-]+)?$`)

	// Docker image: [registry[:port]/]name[:tag][@digest]
	dockerImageRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:-]*(@sha256:[a-f0-9]{64})?$`)
)

// ValidatePluginID checks that a plugin ID is safe to use as a directory name
func ValidatePluginID(id string) error {
	if !pluginIDRe.MatchString(id) {
		return fmt.Errorf("invalid plugin ID %q: use letters, numbers, '.', '_' and '-' (starting with a letter or number)", id)
	}
	return nil
}

// ValidatePackage checks a plugin's package before it's passed to an
// installer or docker: it can't be read as an option or contain spaces.
// npm and pip packages stay flexible (versions, scopes, URLs); Go packages
// and docker images must have their usual form.
func ValidatePackage(installType, pkg string) error {
	switch installType {
	case "npm", "pip", "go", "docker":
	default:
		return nil // Not passed to an installer
	}

	if pkg == "" {
		if installType == "docker" {
			return nil // Custom docker plugin with its own startup command
		}
		return fmt.Errorf("a package is required for %s plugins", installType)
	}
	if strings.HasPrefix(pkg, "-") {
		return fmt.Errorf("%q can't be used as a %s package: packages can't start with \"-\"", pkg, installType)
	}
	if strings.IndexFunc(pkg, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return fmt.Errorf("%q can't be used as a %s package: it contains spaces or control characters", pkg, installType)
	}

	switch installType {
	case "go":
		if !goPackageRe.MatchString(pkg) {
			return fmt.Errorf("%q isn't a Go package path (e.g. github.com/user/repo/cmd/server@latest)", pkg)
		}
	case "docker":
		if !dockerImageRe.MatchString(pkg) {
			return fmt.Errorf("%q isn't a docker image (e.g. ghcr.io/user/image:tag)", pkg)
		}
	}
	return nil
}

// insideDir reports whether path is base or inside it
func insideDir(base, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// pluginDir returns the directory of a plugin, refusing IDs that would
// point outside the plugins directory
func (i *Installer) pluginDir(id string) (string, error) {
	if err := ValidatePluginID(id); err != nil {
		return "", err
	}
	dir := filepath.Join(i.pluginsDir, id)
	if !insideDir(i.pluginsDir, dir) || dir == filepath.Clean(i.pluginsDir) {
		return "", fmt.Errorf("invalid plugin ID %q", id)
	}
	return dir, nil
}
