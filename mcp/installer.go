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

	pluginDir := filepath.Join(i.pluginsDir, pluginID)
	if err := os.RemoveAll(pluginDir); err != nil {
		return fmt.Errorf("failed to remove plugin directory: %w", err)
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

func (i *Installer) installNPX(plugin *Plugin, progressCh chan<- InstallProgress) error {
	pluginDir := filepath.Join(i.pluginsDir, plugin.ID)
	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "installing", Percent: 30, Message: "Installing npm package..."}
	}

	cmd := exec.Command("npm", "install", plugin.Package)
	cmd.Dir = pluginDir
	if output, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(pluginDir)
		return fmt.Errorf("npm install failed: %w\n%s", err, string(output))
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "saving", Percent: 70, Message: "Saving metadata..."}
	}

	now := time.Now()
	installed := storage.InstalledPlugin{
		ID:            plugin.ID,
		Name:          plugin.Name,
		Version:       plugin.Package,
		InstallPath:   pluginDir,
		InstallMethod: "npx",
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
		progressCh <- InstallProgress{Stage: "complete", Percent: 100, Message: "Installation complete"}
	}

	return nil
}

func (i *Installer) installPip(plugin *Plugin, progressCh chan<- InstallProgress) error {
	pluginDir := filepath.Join(i.pluginsDir, plugin.ID)
	venvPath := filepath.Join(pluginDir, "venv")

	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "venv", Percent: 30, Message: "Creating virtual environment..."}
	}

	cmd := exec.Command("python3", "-m", "venv", venvPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("venv creation failed: %w", err)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "pip", Percent: 60, Message: "Installing package..."}
	}

	pipPath := filepath.Join(venvPath, "bin", "pip")
	cmd = exec.Command(pipPath, "install", plugin.Package)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pip install failed: %w\n%s", err, string(output))
	}

	now := time.Now()
	installed := storage.InstalledPlugin{
		ID:            plugin.ID,
		Name:          plugin.Name,
		Version:       plugin.Package,
		InstallPath:   pluginDir,
		InstallMethod: "pip",
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
		progressCh <- InstallProgress{Stage: "complete", Percent: 100, Message: "Installation complete"}
	}

	return nil
}

func (i *Installer) installGo(plugin *Plugin, progressCh chan<- InstallProgress) error {
	pluginDir := filepath.Join(i.pluginsDir, plugin.ID)
	binPath := filepath.Join(pluginDir, "bin")

	if err := os.MkdirAll(binPath, 0700); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "building", Percent: 40, Message: "Building Go binary..."}
	}

	cmd := exec.Command("go", "install", plugin.Package)
	cmd.Env = append(os.Environ(), "GOBIN="+binPath)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go install failed: %w\n%s", err, string(output))
	}

	now := time.Now()
	installed := storage.InstalledPlugin{
		ID:            plugin.ID,
		Name:          plugin.Name,
		Version:       plugin.Package,
		InstallPath:   pluginDir,
		InstallMethod: "go",
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
		progressCh <- InstallProgress{Stage: "complete", Percent: 100, Message: "Installation complete"}
	}

	return nil
}

func (i *Installer) installBinary(plugin *Plugin, progressCh chan<- InstallProgress) error {
	pluginDir := filepath.Join(i.pluginsDir, plugin.ID)
	binDir := filepath.Join(pluginDir, "bin")

	if err := os.MkdirAll(binDir, 0700); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	now := time.Now()
	installed := storage.InstalledPlugin{
		ID:            plugin.ID,
		Name:          plugin.Name,
		Version:       "binary",
		InstallPath:   pluginDir,
		InstallMethod: "binary",
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
		progressCh <- InstallProgress{Stage: "complete", Percent: 100, Message: "Ready for manual binary setup"}
	}

	return nil
}

func (i *Installer) installManual(plugin *Plugin, progressCh chan<- InstallProgress) error {
	pluginDir := filepath.Join(i.pluginsDir, plugin.ID)
	binDir := filepath.Join(pluginDir, "bin")

	if err := os.MkdirAll(binDir, 0700); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	now := time.Now()
	installed := storage.InstalledPlugin{
		ID:            plugin.ID,
		Name:          plugin.Name,
		Version:       "manual",
		InstallPath:   pluginDir,
		InstallMethod: "manual",
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
		progressCh <- InstallProgress{Stage: "complete", Percent: 100, Message: "Ready for manual setup"}
	}

	return nil
}

func (i *Installer) GetRuntimeChecker() *RuntimeChecker {
	return i.runtimeChecker
}

func (i *Installer) installNPXWithContext(ctx context.Context, plugin *Plugin, progressCh chan<- InstallProgress) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("installation cancelled")
	default:
	}

	pluginDir := filepath.Join(i.pluginsDir, plugin.ID)
	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "installing", Percent: 30, Message: "Installing npm package..."}
	}

	cmd := exec.CommandContext(ctx, "npm", "install", plugin.Package)
	cmd.Dir = pluginDir
	if output, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(pluginDir)
		if ctx.Err() != nil {
			return fmt.Errorf("installation cancelled")
		}
		return fmt.Errorf("npm install failed: %w\n%s", err, string(output))
	}

	return i.installNPX(plugin, progressCh)
}

func (i *Installer) installPipWithContext(ctx context.Context, plugin *Plugin, progressCh chan<- InstallProgress) error {
	select {
	case <-ctx.Done():
		pluginDir := filepath.Join(i.pluginsDir, plugin.ID)
		os.RemoveAll(pluginDir)
		return fmt.Errorf("installation cancelled")
	default:
	}

	pluginDir := filepath.Join(i.pluginsDir, plugin.ID)
	venvPath := filepath.Join(pluginDir, "venv")

	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "venv", Percent: 30, Message: "Creating virtual environment..."}
	}

	cmd := exec.CommandContext(ctx, "python3", "-m", "venv", venvPath)
	if err := cmd.Run(); err != nil {
		os.RemoveAll(pluginDir)
		if ctx.Err() != nil {
			return fmt.Errorf("installation cancelled")
		}
		return fmt.Errorf("venv creation failed: %w", err)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "pip", Percent: 60, Message: "Installing package..."}
	}

	pipPath := filepath.Join(venvPath, "bin", "pip")
	cmd = exec.CommandContext(ctx, pipPath, "install", plugin.Package)
	if output, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(pluginDir)
		if ctx.Err() != nil {
			return fmt.Errorf("installation cancelled")
		}
		return fmt.Errorf("pip install failed: %w\n%s", err, string(output))
	}

	now := time.Now()
	installed := storage.InstalledPlugin{
		ID:            plugin.ID,
		Name:          plugin.Name,
		Version:       plugin.Package,
		InstallPath:   pluginDir,
		InstallMethod: "pip",
		InstalledAt:   now,
		UpdatedAt:     now,
	}

	if err := i.storage.Save(installed); err != nil {
		os.RemoveAll(pluginDir)
		return fmt.Errorf("failed to save plugin metadata: %w", err)
	}

	freshConfig, err := config.LoadPluginsConfig(i.dataDir)
	if err == nil {
		freshConfig.SetPluginEnabled(plugin.ID, false)
		_ = config.SavePluginsConfig(i.dataDir, freshConfig)
		i.pluginsConfig = freshConfig
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "complete", Percent: 100, Message: "Installation complete"}
	}

	return nil
}

func (i *Installer) installGoWithContext(ctx context.Context, plugin *Plugin, progressCh chan<- InstallProgress) error {
	select {
	case <-ctx.Done():
		pluginDir := filepath.Join(i.pluginsDir, plugin.ID)
		os.RemoveAll(pluginDir)
		return fmt.Errorf("installation cancelled")
	default:
	}

	pluginDir := filepath.Join(i.pluginsDir, plugin.ID)
	binPath := filepath.Join(pluginDir, "bin")

	if err := os.MkdirAll(binPath, 0700); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "building", Percent: 40, Message: "Building Go binary..."}
	}

	cmd := exec.CommandContext(ctx, "go", "install", plugin.Package)
	cmd.Env = append(os.Environ(), "GOBIN="+binPath)

	if output, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(pluginDir)
		if ctx.Err() != nil {
			return fmt.Errorf("installation cancelled")
		}
		return fmt.Errorf("go install failed: %w\n%s", err, string(output))
	}

	now := time.Now()
	installed := storage.InstalledPlugin{
		ID:            plugin.ID,
		Name:          plugin.Name,
		Version:       plugin.Package,
		InstallPath:   pluginDir,
		InstallMethod: "go",
		InstalledAt:   now,
		UpdatedAt:     now,
	}

	if err := i.storage.Save(installed); err != nil {
		os.RemoveAll(pluginDir)
		return fmt.Errorf("failed to save plugin metadata: %w", err)
	}

	freshConfig, err := config.LoadPluginsConfig(i.dataDir)
	if err == nil {
		freshConfig.SetPluginEnabled(plugin.ID, false)
		_ = config.SavePluginsConfig(i.dataDir, freshConfig)
		i.pluginsConfig = freshConfig
	}

	if progressCh != nil {
		progressCh <- InstallProgress{Stage: "complete", Percent: 100, Message: "Installation complete"}
	}

	return nil
}

func (i *Installer) installBinaryWithContext(ctx context.Context, plugin *Plugin, progressCh chan<- InstallProgress) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("installation cancelled")
	default:
	}
	return i.installBinary(plugin, progressCh)
}

func (i *Installer) installManualWithContext(ctx context.Context, plugin *Plugin, progressCh chan<- InstallProgress) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("installation cancelled")
	default:
	}
	return i.installManual(plugin, progressCh)
}
