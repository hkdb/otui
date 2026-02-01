package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// KeyBindingsConfig holds modifier customization and optional per-action overrides
type KeyBindingsConfig struct {
	Modifiers ModifierConfig    `toml:"modifiers"`
	Actions   map[string]string `toml:"actions"` // Optional overrides for specific actions
}

type ModifierConfig struct {
	Primary   string `toml:"primary"`   // e.g., "alt", "ctrl", "meta", "super"
	Secondary string `toml:"secondary"` // e.g., "alt+shift", "ctrl+shift"
}

// actionDef defines the default modifier and key for an action
type actionDef struct {
	modifier string // "primary", "secondary", or "none"
	key      string // "j", "k", "enter", etc.
}

// actionRegistry maps action names to their default keybindings
// Users can override any of these in the [actions] section of keybindings.toml
var actionRegistry = map[string]actionDef{
	// Main view - Modal toggles
	"help":                  {"primary", "h"},
	"new_session":           {"primary", "n"},
	"session_manager":       {"primary", "s"},
	"edit_session":          {"primary", "e"},
	"model_selector":        {"primary", "m"},
	"search_messages":       {"primary", "f"},    // Search current session
	"search_all_sessions":   {"secondary", "f"},  // Search all sessions (global)
	"plugin_manager":        {"primary", "p"},    // Plugin manager
	"about":                 {"secondary", "a"},
	"settings":              {"secondary", "s"},

	// Main view - Scrolling
	"scroll_down":           {"primary", "j"},
	"scroll_up":             {"primary", "k"},
	"scroll_down_arrow":     {"primary", "down"},
	"scroll_up_arrow":       {"primary", "up"},
	"half_page_down":        {"secondary", "j"},
	"half_page_up":          {"secondary", "k"},
	"half_page_down_arrow":  {"secondary", "down"},
	"half_page_up_arrow":    {"secondary", "up"},
	"page_down":             {"primary", "pgdown"},
	"page_up":               {"primary", "pgup"},
	"scroll_to_top":         {"primary", "g"},
	"scroll_to_bottom":      {"secondary", "g"},

	// Main view - Actions
	"quit":                  {"primary", "q"},
	"yank_last_response":    {"primary", "y"},
	"yank_conversation":     {"primary", "c"},
	"external_editor":       {"primary", "i"},

	// Model selector modal - normal mode (no modifier needed)
	"model_selector_down":       {"none", "j"},
	"model_selector_up":         {"none", "k"},
	"model_selector_down_arrow": {"none", "down"},
	"model_selector_up_arrow":   {"none", "up"},

	// Model selector modal - filter mode (modifier required)
	"model_selector_down_filtered":       {"primary", "j"},
	"model_selector_up_filtered":         {"primary", "k"},
	"model_selector_down_arrow_filtered": {"primary", "down"},
	"model_selector_up_arrow_filtered":   {"primary", "up"},

	// Model selector modal - other actions
	"model_selector_refresh": {"primary", "r"},
	"close_model_selector":   {"primary", "m"},

	// Plugin manager modal - normal mode (no modifier needed)
	"plugin_down":       {"none", "j"},
	"plugin_up":         {"none", "k"},
	"plugin_down_arrow": {"none", "down"},
	"plugin_up_arrow":   {"none", "up"},

	// Plugin manager modal - filter mode (modifier required)
	"plugin_down_filtered":       {"primary", "j"},
	"plugin_up_filtered":         {"primary", "k"},
	"plugin_down_arrow_filtered": {"primary", "down"},
	"plugin_up_arrow_filtered":   {"primary", "up"},

	// Plugin manager modal - other actions (normal mode)
	"plugin_install":  {"none", "i"},
	"plugin_refresh":  {"primary", "r"},

	// Plugin manager modal - other actions (filter mode)
	"plugin_install_filtered": {"primary", "i"},

	// Provider settings modal (accessed from Settings - no modifiers needed)
	"provider_down":       {"none", "j"},
	"provider_up":         {"none", "k"},
	"provider_down_arrow": {"none", "down"},
	"provider_up_arrow":   {"none", "up"},

	// Settings modal (no modifiers needed for navigation)
	"settings_down":       {"none", "j"},
	"settings_up":         {"none", "k"},
	"settings_down_arrow": {"none", "down"},
	"settings_up_arrow":   {"none", "up"},

	// Universal clear input action (works in all text input contexts)
	"clear_input": {"primary", "u"},

	// About modal
	"close_about": {"primary", "a"},

	// Welcome wizard (inherits modifiers - wizard runs before main app)
	"welcome_down":       {"primary", "j"},
	"welcome_up":         {"primary", "k"},
	"welcome_down_arrow": {"primary", "down"},
	"welcome_up_arrow":   {"primary", "up"},
	"welcome_quit":       {"primary", "q"},
}

// DefaultKeybindings returns default configuration
func DefaultKeybindings() *KeyBindingsConfig {
	return &KeyBindingsConfig{
		Modifiers: ModifierConfig{
			Primary:   "alt",
			Secondary: "alt+shift",
		},
	}
}

// LoadKeybindings loads keybindings from data directory
func LoadKeybindings(dataDir string) (*KeyBindingsConfig, error) {
	cfg := DefaultKeybindings()
	keybindingsPath := filepath.Join(dataDir, "keybindings.toml")

	if !FileExists(keybindingsPath) {
		if err := CreateDefaultKeybindings(dataDir); err != nil {
			return nil, fmt.Errorf("failed to create keybindings: %w", err)
		}
		return cfg, nil
	}

	_, err := toml.DecodeFile(keybindingsPath, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse keybindings: %w", err)
	}

	// Validate and apply defaults if missing
	if cfg.Modifiers.Primary == "" {
		cfg.Modifiers.Primary = "alt"
	}
	if cfg.Modifiers.Secondary == "" {
		cfg.Modifiers.Secondary = "alt+shift"
	}

	return cfg, nil
}

// CreateDefaultKeybindings creates default keybindings.toml
func CreateDefaultKeybindings(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	keybindingsPath := filepath.Join(dataDir, "keybindings.toml")
	if FileExists(keybindingsPath) {
		return nil
	}

	content := GenerateKeybindingsTemplate()
	if err := os.WriteFile(keybindingsPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write keybindings: %w", err)
	}

	return nil
}

// GenerateKeybindingsTemplate returns the default TOML template
func GenerateKeybindingsTemplate() string {
	return `# OTUI Keybindings Configuration
# Location: ~/.local/share/otui/keybindings.toml
# This file uses TOML format: https://toml.io

# ==============================================================================
# MODIFIER KEYS (Simple Configuration)
# ==============================================================================
# Change these to avoid conflicts with your window manager/terminal multiplexor
# Most users only need to customize these two settings

[modifiers]
primary = "alt"          # Default: alt (Options: alt, ctrl, meta, super)
secondary = "alt+shift"  # Default: alt+shift

# Examples of alternative modifier configurations:
#
# For tmux users (Alt may conflict):
#   primary = "ctrl"
#   secondary = "ctrl+shift"
#
# For i3/sway users (Alt is window manager key):
#   primary = "super"
#   secondary = "super+shift"
#
# Mixed modifiers for power users:
#   primary = "alt"
#   secondary = "ctrl+shift"

# ==============================================================================
# PER-ACTION OVERRIDES (Advanced Configuration)
# ==============================================================================
# Optionally override specific actions for fine-grained control
# Uncomment and customize any actions you want to change
# See docs/KEYBINDINGS.md for a complete list of available actions

[actions]
# Examples (uncomment to use):
#
# Vim-style navigation with Ctrl:
#   scroll_down = "ctrl+j"
#   scroll_up = "ctrl+k"
#
# Emacs-style shortcuts:
#   scroll_down = "ctrl+n"
#   scroll_up = "ctrl+p"
#
# Custom session management:
#   new_session = "ctrl+t"
#   session_manager = "ctrl+shift+s"
#
# Remap quit to avoid accidental exits:
#   quit = "ctrl+shift+q"

# Complete action list available in https://github.com/hkdb/otui/blob/main/docs/KEYBINDINGS.md
`
}

// Helper methods for building keybinding strings

// Primary returns the primary modifier
func (kb *KeyBindingsConfig) Primary() string {
	if kb.Modifiers.Primary == "" {
		return "alt"
	}
	return kb.Modifiers.Primary
}

// Secondary returns the secondary modifier
func (kb *KeyBindingsConfig) Secondary() string {
	if kb.Modifiers.Secondary == "" {
		return "alt+shift"
	}
	return kb.Modifiers.Secondary
}

// PrimaryKey builds a keybinding string with primary modifier
// Example: PrimaryKey("s") returns "alt+s" (or "ctrl+s" if primary is "ctrl")
func (kb *KeyBindingsConfig) PrimaryKey(key string) string {
	return kb.Primary() + "+" + key
}

// SecondaryKey builds a keybinding string with secondary modifier
// For modifiers containing "shift" + single letter keys, returns uppercase letter
// Example: SecondaryKey("s") returns "alt+S" (not "alt+shift+s")
// Example: SecondaryKey("f1") returns "alt+shift+f1" (special keys keep explicit shift)
func (kb *KeyBindingsConfig) SecondaryKey(key string) string {
	secondary := kb.Secondary()

	// If secondary contains "shift" and key is a single lowercase letter,
	// use uppercase letter instead of explicit shift (matches terminal behavior)
	if strings.Contains(strings.ToLower(secondary), "shift") && len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
		// Remove "shift" from modifier and use uppercase letter
		modParts := strings.Split(secondary, "+")
		var cleanMods []string
		for _, part := range modParts {
			if strings.ToLower(part) != "shift" {
				cleanMods = append(cleanMods, part)
			}
		}
		if len(cleanMods) > 0 {
			return strings.Join(cleanMods, "+") + "+" + strings.ToUpper(key)
		}
		return strings.ToUpper(key)
	}

	return secondary + "+" + key
}

// PrimaryDisplay returns capitalized modifier for display in UI
// Example: "alt" -> "Alt", "ctrl" -> "Ctrl"
func (kb *KeyBindingsConfig) PrimaryDisplay() string {
	return capitalizeModifier(kb.Primary())
}

// SecondaryDisplay returns capitalized modifier for display in UI
func (kb *KeyBindingsConfig) SecondaryDisplay() string {
	return capitalizeModifier(kb.Secondary())
}

// Helper to capitalize modifier strings for display
func capitalizeModifier(mod string) string {
	parts := strings.Split(mod, "+")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "+")
}

// GetActionKey returns the keybinding for a specific action
// Checks user overrides first, then falls back to action registry defaults
// Example: GetActionKey("scroll_down") returns "alt+j" (or user override like "ctrl+d")
func (kb *KeyBindingsConfig) GetActionKey(action string) string {
	// Check for user override first
	if kb.Actions != nil {
		if override, exists := kb.Actions[action]; exists && override != "" {
			return override
		}
	}

	// Fall back to action registry default
	if def, exists := actionRegistry[action]; exists {
		switch def.modifier {
		case "primary":
			return kb.PrimaryKey(def.key)
		case "secondary":
			return kb.SecondaryKey(def.key)
		case "none":
			return def.key
		}
	}

	// Action not found - return empty string
	return ""
}

// DisplayActionKey returns a display-friendly version of an action's keybinding
// Example: "ctrl+shift+j" -> "Ctrl+Shift+J"
func (kb *KeyBindingsConfig) DisplayActionKey(action string) string {
	key := kb.GetActionKey(action)
	if key == "" {
		return ""
	}
	return capitalizeKeybinding(key)
}

// capitalizeKeybinding capitalizes a keybinding string for display
// Converts uppercase letters to Shift+ format for clarity
// Examples:
//   "ctrl+shift+j" -> "Ctrl+Shift+J"
//   "alt+D" -> "Alt+Shift+D" (uppercase D = Shift+D)
//   "alt+j" -> "Alt+J"
func capitalizeKeybinding(key string) string {
	parts := strings.Split(key, "+")
	var result []string

	for i, part := range parts {
		if len(part) == 0 {
			continue
		}

		// Check if this is a single uppercase letter (indicates Shift was pressed)
		if len(part) == 1 && part[0] >= 'A' && part[0] <= 'Z' {
			// Insert "Shift+" before the uppercase letter and convert to lowercase
			// But only if "shift" isn't already in the parts
			hasShift := false
			for _, p := range parts {
				if strings.ToLower(p) == "shift" {
					hasShift = true
					break
				}
			}
			if !hasShift && i > 0 { // Only add shift if there's a modifier before it
				result = append(result, "Shift")
			}
			result = append(result, strings.ToUpper(part[:1]))
		} else {
			// Regular part - just capitalize first letter
			result = append(result, strings.ToUpper(part[:1])+part[1:])
		}
	}

	return strings.Join(result, "+")
}

// Validate checks if the configuration is valid
// Returns (isValid, warningMessage)
func (kb *KeyBindingsConfig) Validate() (bool, string) {
	primary := kb.Primary()
	secondary := kb.Secondary()

	// Disallow empty
	if primary == "" || secondary == "" {
		return false, "Modifiers cannot be empty"
	}

	// Disallow shift alone
	if primary == "shift" || secondary == "shift" {
		return false, "Shift alone conflicts with typing"
	}

	// Warn about ctrl usage (but allow it)
	if strings.Contains(primary, "ctrl") || strings.Contains(secondary, "ctrl") {
		return true, "Warning: Ctrl may conflict with terminal shortcuts (Ctrl+C, Ctrl+Z, Ctrl+D)"
	}

	return true, ""
}
