package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"otui/config"
	"otui/storage"
)

type Installer struct {
	storage        *storage.PluginStorage
	pluginsConfig  *config.PluginsConfig
	runtimeChecker *RuntimeChecker
	pluginsDir     string
	dataDir        string
}

type InstallProgress struct {
	Stage   string
	Percent float64
	Message string
}

func NewInstaller(pluginStorage *storage.PluginStorage, pluginsConfig *config.PluginsConfig, dataDir string) *Installer {
	return &Installer{
		storage:        pluginStorage,
		pluginsConfig:  pluginsConfig,
		runtimeChecker: NewRuntimeChecker(),
		pluginsDir:     filepath.Join(dataDir, "plugins"),
		dataDir:        dataDir,
	}
}

func (i *Installer) Install(plugin *Plugin, progressCh chan<- InstallProgress) error {
	return i.InstallWithContext(context.Background(), plugin, progressCh)
}

func (i *Installer) InstallWithContext(ctx context.Context, plugin *Plugin, progressCh chan<- InstallProgress) error {
	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "checking", Percent: 0, Message: "Checking requirements..."}
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("installation cancelled")
	default:
	}

	// Registry and custom plugin data is untrusted: it becomes paths and
	// command arguments
	if err := ValidatePluginID(plugin.ID); err != nil {
		return err
	}
	if err := ValidatePackage(plugin.InstallType, plugin.Package); err != nil {
		return err
	}

	if err := i.checkRuntimeDeps(plugin); err != nil {
		return fmt.Errorf("runtime check failed: %w", err)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "installing", Percent: 20, Message: "Installing plugin..."}
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("installation cancelled")
	default:
	}

	switch plugin.InstallType {
	case "npm":
		return i.installNPXWithContext(ctx, plugin, progressCh)
	case "pip":
		return i.installPipWithContext(ctx, plugin, progressCh)
	case "go":
		return i.installGoWithContext(ctx, plugin, progressCh)
	case "binary":
		return i.installBinaryWithContext(ctx, plugin, progressCh)
	case "remote":
		return i.installRemoteWithContext(ctx, plugin, progressCh)
	case "docker":
		return i.installDockerWithContext(ctx, plugin, progressCh)
	default:
		// Treat all other install types (manual, docker, uvx, etc.) as manual installation
		return i.installManualWithContext(ctx, plugin, progressCh)
	}
}

func (i *Installer) Uninstall(pluginID string) error {
	return i.UninstallWithContext(context.Background(), pluginID, nil)
}

func (i *Installer) UninstallWithContext(ctx context.Context, pluginID string, progressCh chan<- InstallProgress) error {
	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "starting", Percent: 0, Message: "Starting uninstall..."}
	}

	installed, err := i.storage.Load(pluginID)
	if err != nil {
		return fmt.Errorf("failed to load plugin: %w", err)
	}
	if installed == nil {
		return fmt.Errorf("plugin not installed")
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "loading", Percent: 25, Message: "Loading plugin metadata..."}
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "removing", Percent: 50, Message: "Removing files..."}
	}

	// An invalid ID (e.g. "..") must not delete outside the plugins
	// directory: only its records are removed
	if pluginDir, err := i.pluginDir(pluginID); err == nil {
		if err := os.RemoveAll(pluginDir); err != nil {
			return fmt.Errorf("failed to remove plugin directory: %w", err)
		}
	} else if config.DebugLog != nil {
		config.DebugLog.Printf("[MCP] Uninstall: not removing files of plugin with invalid ID %q", pluginID)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "database", Percent: 75, Message: "Updating database..."}
	}

	if err := i.storage.Delete(pluginID); err != nil {
		return fmt.Errorf("failed to remove from database: %w", err)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "config", Percent: 90, Message: "Updating configuration..."}
	}

	freshConfig, err := config.LoadPluginsConfig(i.dataDir)
	if err == nil {
		freshConfig.DeletePlugin(pluginID)
		_ = config.SavePluginsConfig(i.dataDir, freshConfig)
		i.pluginsConfig = freshConfig
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "complete", Percent: 100, Message: "Uninstall complete"}
	}

	return nil
}

func (i *Installer) IsInstalled(pluginID string) bool {
	return i.storage.IsInstalled(pluginID)
}

func (i *Installer) GetInstalled(pluginID string) (*storage.InstalledPlugin, error) {
	return i.storage.Load(pluginID)
}

func (i *Installer) ListInstalled() ([]storage.InstalledPlugin, error) {
	return i.storage.List()
}

func (i *Installer) checkRuntimeDeps(plugin *Plugin) error {
	switch plugin.InstallType {
	case "remote":
		return nil // No runtime dependencies for remote plugins
	case "npm":
		if _, err := i.runtimeChecker.CheckRuntime("node"); err != nil {
			return fmt.Errorf("Node.js is required: %w", err)
		}
		if _, err := i.runtimeChecker.CheckRuntime("npx"); err != nil {
			return fmt.Errorf("npx is required: %w", err)
		}

		for _, dep := range plugin.RuntimeDeps {
			if strings.HasPrefix(dep, "node>=") {
				minVersion := strings.TrimPrefix(dep, "node>=")
				if err := i.runtimeChecker.CheckVersion("node", minVersion); err != nil {
					return err
				}
			}
		}

	case "pip":
		if _, err := i.runtimeChecker.CheckRuntime("python"); err != nil {
			return fmt.Errorf("Python with venv is required: %w", err)
		}

		for _, dep := range plugin.RuntimeDeps {
			if strings.HasPrefix(dep, "python>=") {
				minVersion := strings.TrimPrefix(dep, "python>=")
				if err := i.runtimeChecker.CheckVersion("python", minVersion); err != nil {
					return err
				}
			}
		}

	case "docker":
		// A custom docker plugin with its own startup command and no image
		// is run as configured
		if plugin.Package != "" {
			if _, err := i.runtimeChecker.CheckRuntime("docker"); err != nil {
				return fmt.Errorf("Docker is required: %w", err)
			}
		}

	case "go":
		if _, err := i.runtimeChecker.CheckRuntime("go"); err != nil {
			return fmt.Errorf("Go is required: %w", err)
		}

		for _, dep := range plugin.RuntimeDeps {
			if strings.HasPrefix(dep, "go>=") {
				minVersion := strings.TrimPrefix(dep, "go>=")
				if err := i.runtimeChecker.CheckVersion("go", minVersion); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// recordInstall saves an installed plugin's metadata, disabled by default
// (like every install), and reports completion
func (i *Installer) recordInstall(plugin *Plugin, pluginDir, version, method, doneMessage string, progressCh chan<- InstallProgress) error {
	now := time.Now()
	installed := storage.InstalledPlugin{
		ID:            plugin.ID,
		Name:          plugin.Name,
		Version:       version,
		InstallPath:   pluginDir,
		InstallMethod: method,
		InstalledAt:   now,
		UpdatedAt:     now,
	}

	if err := i.storage.Save(installed); err != nil {
		return fmt.Errorf("failed to save plugin metadata: %w", err)
	}

	freshConfig, err := config.LoadPluginsConfig(i.dataDir)
	if err == nil {
		freshConfig.SetPluginEnabled(plugin.ID, false)
		_ = config.SavePluginsConfig(i.dataDir, freshConfig)
		i.pluginsConfig = freshConfig
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "complete", Percent: 100, Message: doneMessage}
	}
	return nil
}

// installFailed removes a partly installed plugin and describes the error
func installFailed(ctx context.Context, pluginDir, what string, err error, output []byte) error {
	os.RemoveAll(pluginDir)
	if ctx.Err() != nil {
		return fmt.Errorf("installation cancelled")
	}
	if len(output) > 0 {
		return fmt.Errorf("%s failed: %w\n%s", what, err, string(output))
	}
	return fmt.Errorf("%s failed: %w", what, err)
}

func (i *Installer) GetRuntimeChecker() *RuntimeChecker {
	return i.runtimeChecker
}

func (i *Installer) installNPXWithContext(ctx context.Context, plugin *Plugin, progressCh chan<- InstallProgress) error {
	if ctx.Err() != nil {
		return fmt.Errorf("installation cancelled")
	}

	pluginDir, err := i.pluginDir(plugin.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "installing", Percent: 30, Message: "Installing npm package..."}
	}

	// "--": the package can't be read as an npm option
	cmd := exec.CommandContext(ctx, "npm", "install", "--", plugin.Package)
	cmd.Dir = pluginDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return installFailed(ctx, pluginDir, "npm install", err, output)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "saving", Percent: 70, Message: "Saving metadata..."}
	}
	if err := i.recordInstall(plugin, pluginDir, plugin.Package, "npx", "Installation complete", progressCh); err != nil {
		os.RemoveAll(pluginDir)
		return err
	}
	return nil
}

func (i *Installer) installPipWithContext(ctx context.Context, plugin *Plugin, progressCh chan<- InstallProgress) error {
	pluginDir, err := i.pluginDir(plugin.ID)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		os.RemoveAll(pluginDir)
		return fmt.Errorf("installation cancelled")
	}

	venvPath := filepath.Join(pluginDir, "venv")
	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "venv", Percent: 30, Message: "Creating virtual environment..."}
	}

	if err := exec.CommandContext(ctx, "python3", "-m", "venv", venvPath).Run(); err != nil {
		return installFailed(ctx, pluginDir, "venv creation", err, nil)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "pip", Percent: 60, Message: "Installing package..."}
	}

	// "--": the package can't be read as a pip option
	pipPath := filepath.Join(venvPath, "bin", "pip")
	if output, err := exec.CommandContext(ctx, pipPath, "install", "--", plugin.Package).CombinedOutput(); err != nil {
		return installFailed(ctx, pluginDir, "pip install", err, output)
	}

	if err := i.recordInstall(plugin, pluginDir, plugin.Package, "pip", "Installation complete", progressCh); err != nil {
		os.RemoveAll(pluginDir)
		return err
	}
	return nil
}

func (i *Installer) installGoWithContext(ctx context.Context, plugin *Plugin, progressCh chan<- InstallProgress) error {
	pluginDir, err := i.pluginDir(plugin.ID)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		os.RemoveAll(pluginDir)
		return fmt.Errorf("installation cancelled")
	}

	binPath := filepath.Join(pluginDir, "bin")
	if err := os.MkdirAll(binPath, 0700); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "building", Percent: 40, Message: "Building Go binary..."}
	}

	// "--": the package can't be read as a go flag (e.g. -toolexec)
	cmd := exec.CommandContext(ctx, "go", "install", "--", plugin.Package)
	cmd.Env = append(os.Environ(), "GOBIN="+binPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return installFailed(ctx, pluginDir, "go install", err, output)
	}

	if err := i.recordInstall(plugin, pluginDir, plugin.Package, "go", "Installation complete", progressCh); err != nil {
		os.RemoveAll(pluginDir)
		return err
	}
	return nil
}

// installDirOnly creates the plugin's directory (with bin/) and records the
// install, for plugins set up by hand (binary, manual)
func (i *Installer) installDirOnly(ctx context.Context, plugin *Plugin, version, doneMessage string, progressCh chan<- InstallProgress) error {
	if ctx.Err() != nil {
		return fmt.Errorf("installation cancelled")
	}

	pluginDir, err := i.pluginDir(plugin.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(pluginDir, "bin"), 0700); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}
	return i.recordInstall(plugin, pluginDir, version, version, doneMessage, progressCh)
}

func (i *Installer) installBinaryWithContext(ctx context.Context, plugin *Plugin, progressCh chan<- InstallProgress) error {
	return i.installDirOnly(ctx, plugin, "binary", "Ready for manual binary setup", progressCh)
}

// installDockerWithContext pulls a docker plugin's image. Images that can't
// be pulled are accepted if they exist locally (e.g. built from the plugin's
// repository).
func (i *Installer) installDockerWithContext(ctx context.Context, plugin *Plugin, progressCh chan<- InstallProgress) error {
	image := strings.TrimSpace(plugin.Package)
	if image == "" {
		// Custom plugin with its own startup command
		return i.installManualWithContext(ctx, plugin, progressCh)
	}

	pluginDir, err := i.pluginDir(plugin.ID)
	if err != nil {
		return err
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "installing", Percent: 40, Message: fmt.Sprintf("Pulling %s...", image)}
	}

	// "--": the image can't be read as a docker option
	pullOutput, pullErr := exec.CommandContext(ctx, "docker", "pull", "--", image).CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("installation cancelled")
	}
	if pullErr != nil {
		if err := exec.CommandContext(ctx, "docker", "image", "inspect", "--", image).Run(); err != nil {
			return fmt.Errorf("docker pull %s failed: %w\n%s\nIf this image is built locally, build it first (see the plugin's README)",
				image, pullErr, strings.TrimSpace(string(pullOutput)))
		}
	}

	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}
	return i.recordInstall(plugin, pluginDir, image, "docker", "Installation complete", progressCh)
}

func (i *Installer) installManualWithContext(ctx context.Context, plugin *Plugin, progressCh chan<- InstallProgress) error {
	return i.installDirOnly(ctx, plugin, "manual", "Ready for manual setup", progressCh)
}

func (i *Installer) installRemoteWithContext(ctx context.Context, plugin *Plugin, progressCh chan<- InstallProgress) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("installation cancelled")
	default:
	}

	switch {
	case plugin.ServerURL == "":
		return fmt.Errorf("remote plugins require a server URL")
	}

	switch {
	case progressCh != nil:
		progressCh <- InstallProgress{Stage: "validating", Percent: 50, Message: "Validating URL format..."}
	}

	// Validate URL format
	switch {
	case strings.HasPrefix(plugin.ServerURL, "http://"):
		// OK
	case strings.HasPrefix(plugin.ServerURL, "https://"):
		// OK
	default:
		return fmt.Errorf("server URL must start with http:// or https://")
	}

	// Set default auth type
	switch plugin.AuthType {
	case "":
		plugin.AuthType = "none"
	}

	// NO connection test - runtime errors will surface naturally

	// Save metadata
	now := time.Now()
	installedPlugin := storage.InstalledPlugin{
		ID:            plugin.ID,
		Name:          plugin.Name,
		ServerURL:     plugin.ServerURL,
		AuthType:      plugin.AuthType,
		Transport:     plugin.Transport,
		Version:       "",
		InstallPath:   "",
		InstallMethod: "remote",
		InstalledAt:   now,
		UpdatedAt:     now,
	}

	if err := i.storage.Save(installedPlugin); err != nil {
		return fmt.Errorf("failed to save remote plugin: %w", err)
	}

	// Set plugin disabled by default (like other install types)
	freshConfig, err := config.LoadPluginsConfig(i.dataDir)
	switch {
	case err == nil:
		freshConfig.SetPluginEnabled(plugin.ID, false)
		_ = config.SavePluginsConfig(i.dataDir, freshConfig)
		i.pluginsConfig = freshConfig
	}

	switch {
	case progressCh != nil:
		progressCh <- InstallProgress{Stage: "complete", Percent: 100, Message: "Remote plugin added"}
	}

	return nil
}

// UpdateCustomPlugin updates an existing custom plugin in the database
func (i *Installer) UpdateCustomPlugin(plugin *Plugin) error {
	// Validation
	switch {
	case plugin == nil:
		return fmt.Errorf("plugin cannot be nil")
	case plugin.ID == "":
		return fmt.Errorf("plugin ID cannot be empty")
	}

	// Check if plugin is installed
	if !i.IsInstalled(plugin.ID) {
		return fmt.Errorf("plugin %s is not installed", plugin.ID)
	}

	// Load existing metadata to preserve InstalledAt timestamp
	existing, err := i.storage.Load(plugin.ID)
	if err != nil {
		return fmt.Errorf("failed to load existing plugin: %w", err)
	}

	// Build updated database record
	installedPlugin := storage.InstalledPlugin{
		ID:            plugin.ID,
		Name:          plugin.Name,
		Version:       existing.Version,     // Preserve version
		InstallPath:   existing.InstallPath, // Preserve install path
		InstallMethod: plugin.InstallType,
		ServerURL:     plugin.ServerURL,
		AuthType:      plugin.AuthType,
		Transport:     plugin.Transport,
		InstalledAt:   existing.InstalledAt, // Preserve original install time
		UpdatedAt:     time.Now(),           // Update timestamp
	}

	// Update database
	if err := i.storage.Update(installedPlugin); err != nil {
		return fmt.Errorf("failed to update database: %w", err)
	}

	return nil
}
