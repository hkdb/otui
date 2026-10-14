package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

func LoadSystemConfig() (*SystemConfig, error) {
	cfg := DefaultSystemConfig()
	settingsPath := GetSettingsFilePath()

	if !FileExists(settingsPath) {
		if err := CreateDefaultSystemConfig(); err != nil {
			return nil, fmt.Errorf("failed to create system config: %w", err)
		}
		return cfg, nil
	}

	_, err := toml.DecodeFile(settingsPath, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse system config: %w", err)
	}

	return cfg, nil
}

// SystemConfigExists checks if the system config file exists
// without creating it (unlike LoadSystemConfig which creates if missing)
func SystemConfigExists() bool {
	settingsPath := GetSettingsFilePath()
	return FileExists(settingsPath)
}

func LoadUserConfig(dataDir string) (*UserConfig, error) {
	cfg := DefaultUserConfig()
	userConfigPath := filepath.Join(dataDir, "config.toml")

	if !FileExists(userConfigPath) {
		if err := CreateDefaultUserConfig(dataDir); err != nil {
			return nil, fmt.Errorf("failed to create user config: %w", err)
		}
		return cfg, nil
	}

	// Read file content for migration check
	content, err := os.ReadFile(userConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read user config: %w", err)
	}

	md, err := toml.DecodeFile(userConfigPath, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user config: %w", err)
	}

	// Older config templates put some top-level settings after [ollama], so
	// TOML read them as ollama.* and they were ignored - honor them
	applyMisplacedOllamaKeys(userConfigPath, md, cfg)

	// MIGRATION: Add enable_multi_step if not present (Phase 2 feature)
	if !strings.Contains(string(content), "enable_multi_step") {
		// Insert as top-level keys (before the first table, so TOML doesn't
		// put them in whatever table comes last)
		migration := "# Multi-step execution (Phase 2)\nenable_multi_step = true\nmax_iterations = 10\n\n"
		if err := WriteFileAtomic(userConfigPath, []byte(insertBeforeFirstTable(string(content), migration)), 0600); err == nil {
			if Debug && DebugLog != nil {
				DebugLog.Printf("[Config] Migrated: added enable_multi_step and max_iterations to config")
			}
		}
		// Set in memory (since we already decoded before migration)
		cfg.EnableMultiStep = true
		cfg.MaxIterations = 10
	}

	// REPAIR: Profiles created with the "Custom Settings" wizard before this
	// fix were saved with zero values instead of defaults (most importantly
	// require_approval = false, which runs tools without asking)
	if needsWizardDefaultsRepair(cfg) {
		repairWizardDefaults(cfg)
		if err := SaveUserConfig(cfg, dataDir); err != nil {
			if DebugLog != nil {
				DebugLog.Printf("[Config] Failed to save repaired wizard defaults: %v", err)
			}
		} else if DebugLog != nil {
			DebugLog.Printf("[Config] Repaired wizard defaults: require_approval, enable_multi_step, max_iterations, compaction")
		}
	}

	return cfg, nil
}

// misplacedOllamaKeys are top-level settings that older config templates
// wrote after the [ollama] header
type misplacedOllamaKeys struct {
	Ollama struct {
		DefaultSystemPrompt *string  `toml:"default_system_prompt"`
		PluginsEnabled      *bool    `toml:"plugins_enabled"`
		RequireApproval     *bool    `toml:"require_approval"`
		AllowedTools        []string `toml:"allowed_tools"`
		EnableMultiStep     *bool    `toml:"enable_multi_step"`
		MaxIterations       *int     `toml:"max_iterations"`
		NotifyOnComplete    *bool    `toml:"notify_on_complete"`
	} `toml:"ollama"`
}

// applyMisplacedOllamaKeys applies settings found under [ollama] that belong
// at the top level. A correctly placed top-level key always wins. Only the
// in-memory config changes; the file (and its comments) is left alone.
func applyMisplacedOllamaKeys(path string, md toml.MetaData, cfg *UserConfig) {
	if !md.IsDefined("ollama") {
		return
	}

	var misplaced misplacedOllamaKeys
	if _, err := toml.DecodeFile(path, &misplaced); err != nil {
		return
	}
	o := misplaced.Ollama

	var applied []string
	use := func(key string, isSet bool) bool {
		if !isSet || md.IsDefined(key) {
			return false
		}
		applied = append(applied, key)
		return true
	}

	if use("default_system_prompt", o.DefaultSystemPrompt != nil) {
		cfg.DefaultSystemPrompt = *o.DefaultSystemPrompt
	}
	if use("plugins_enabled", o.PluginsEnabled != nil) {
		cfg.PluginsEnabled = *o.PluginsEnabled
	}
	if use("require_approval", o.RequireApproval != nil) {
		cfg.RequireApproval = *o.RequireApproval
	}
	if use("allowed_tools", o.AllowedTools != nil) {
		cfg.AllowedTools = o.AllowedTools
	}
	if use("enable_multi_step", o.EnableMultiStep != nil) {
		cfg.EnableMultiStep = *o.EnableMultiStep
	}
	if use("max_iterations", o.MaxIterations != nil) {
		cfg.MaxIterations = *o.MaxIterations
	}
	if use("notify_on_complete", o.NotifyOnComplete != nil) {
		cfg.NotifyOnComplete = *o.NotifyOnComplete
	}

	if len(applied) > 0 && DebugLog != nil {
		DebugLog.Printf("[Config] Applied settings found under [ollama] as top-level: %s", strings.Join(applied, ", "))
	}
}

// insertBeforeFirstTable inserts text before the first [table] header in a
// TOML document, so it's parsed as top-level keys. Appends if there's no table.
func insertBeforeFirstTable(content, text string) string {
	offset := 0
	for _, line := range strings.SplitAfter(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			return content[:offset] + text + content[offset:]
		}
		offset += len(line)
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + "\n" + text
}

// needsWizardDefaultsRepair detects the zero values written by the old
// "Custom Settings" wizard. No other code path writes this combination:
// the Settings screen never changes max_iterations, so a user who turns
// tool approval off still has max_iterations = 10.
func needsWizardDefaultsRepair(cfg *UserConfig) bool {
	return !cfg.RequireApproval && !cfg.EnableMultiStep && cfg.MaxIterations == 0
}

// repairWizardDefaults restores the defaults the old wizard failed to write
func repairWizardDefaults(cfg *UserConfig) {
	defaults := DefaultUserConfig()

	cfg.RequireApproval = defaults.RequireApproval
	cfg.EnableMultiStep = defaults.EnableMultiStep
	cfg.MaxIterations = defaults.MaxIterations

	if cfg.Compaction.AutoCompactThreshold == 0 {
		cfg.Compaction.AutoCompactThreshold = defaults.Compaction.AutoCompactThreshold
	}
	if cfg.Compaction.KeepPercentage == 0 {
		cfg.Compaction.KeepPercentage = defaults.Compaction.KeepPercentage
	}
	if cfg.Compaction.WarnAtPercentage == 0 {
		cfg.Compaction.WarnAtPercentage = defaults.Compaction.WarnAtPercentage
	}
}

// LoadUserConfigFromPath loads user config from a specific file path
// Returns nil if the file doesn't exist (not an error)
func LoadUserConfigFromPath(configPath string) (*UserConfig, error) {
	if !FileExists(configPath) {
		return nil, nil
	}

	cfg := &UserConfig{}
	_, err := toml.DecodeFile(configPath, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user config: %w", err)
	}

	return cfg, nil
}

func SaveSystemConfig(cfg *SystemConfig) error {
	configDir := GetConfigDir()
	if err := EnsureDir(configDir); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return fmt.Errorf("failed to encode system config: %w", err)
	}

	// Secure permissions (0600 - contains data directory location)
	if err := WriteFileAtomic(GetSettingsFilePath(), buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write system config file: %w", err)
	}

	return nil
}

func SaveUserConfig(cfg *UserConfig, dataDir string) error {
	// Data dir should already exist with correct perms (0700)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return fmt.Errorf("failed to encode user config: %w", err)
	}

	// Secure permissions (0600 - user configuration data)
	if err := WriteFileAtomic(filepath.Join(dataDir, "config.toml"), buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write user config file: %w", err)
	}

	return nil
}

func CreateDefaultSystemConfig() error {
	configDir := GetConfigDir()
	if err := EnsureDir(configDir); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	settingsPath := GetSettingsFilePath()
	if FileExists(settingsPath) {
		return nil
	}

	content := GenerateSystemConfigTemplate()
	if err := os.WriteFile(settingsPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write system config: %w", err)
	}

	return nil
}

func CreateDefaultUserConfig(dataDir string) error {
	// Data dir should already exist with correct perms (0700)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	userConfigPath := filepath.Join(dataDir, "config.toml")
	if FileExists(userConfigPath) {
		return nil
	}

	content := GenerateUserConfigTemplate()
	if err := os.WriteFile(userConfigPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write user config: %w", err)
	}

	return nil
}
