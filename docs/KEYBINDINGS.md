# OTUI Keybindings Guide

Complete guide to customizing keyboard shortcuts in OTUI.

## Table of Contents

- [Overview](#overview)
- [Configuration File](#configuration-file)
- [Quick Start: Modifier Customization](#quick-start-modifier-customization)
- [Advanced: Per-Action Overrides](#advanced-per-action-overrides)
- [Complete Action Reference](#complete-action-reference)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

OTUI provides two levels of keybinding customization:

1. **Modifier Customization (Simple)** - Change `Alt` to `Ctrl`, `Super`, etc. This changes all keybindings at once and solves most OS/window manager conflicts.

2. **Per-Action Overrides (Advanced)** - Customize individual actions for fine-grained control. Perfect for power users who want Vim-style, Emacs-style, or completely custom mappings.

**Default keybindings:**
- Primary modifier: `Alt`
- Secondary modifier: `Alt+Shift`
- Examples: `Alt+N` (new session), `Alt+J/K` (scroll), `Alt+Shift+J/K` (half-page scroll)

## Configuration File

Keybindings are configured in the `data dir`. By default, it's at:
```
~/.local/share/otui/keybindings.toml
```

The file is automatically created with defaults on first run. You can also bring your own `keybindings.toml` when setting up OTUI.

**File permissions:** `0600` (user read/write only)

## Quick Start: Modifier Customization

For most users, customizing modifiers is all you need.

### Example 1: Tmux Users (Alt Conflicts)

If tmux uses `Alt` as its prefix:

```toml
[modifiers]
primary = "ctrl"
secondary = "ctrl+shift"
```

Now all keybindings use `Ctrl` instead of `Alt`:
- `Ctrl+N` → new session
- `Ctrl+J/K` → scroll
- `Ctrl+Shift+J/K` → half-page scroll

### Example 2: i3/Sway Users (Alt is WM Key)

If your window manager uses `Alt`:

```toml
[modifiers]
primary = "super"
secondary = "super+shift"
```

Now all keybindings use the `Super` (Windows/Command) key.

### Example 3: Mixed Modifiers

You can mix modifiers for different purposes:

```toml
[modifiers]
primary = "alt"
secondary = "ctrl+shift"
```

- Navigation: `Alt+J/K` (easy to reach)
- Power actions: `Ctrl+Shift+F` (harder to accidentally trigger)

### Available Modifiers

- `alt` - Alt/Option key
- `ctrl` - Control key
- `super` - Windows key / Command key (macOS)
- `meta` - Meta key (same as `alt` in terminals)
- `shift` - Should only be used in combinations (not alone)

You can combine modifiers with `+`:
- `ctrl+alt`
- `ctrl+shift`
- `alt+shift`

## Advanced: Per-Action Overrides

Override specific actions while keeping the rest at defaults.

### How It Works

1. OTUI checks if you've overridden an action in `[actions]`
2. If not found, it falls back to `[modifiers]` + default key
3. This lets you customize only what you need

### Example: Vim-Style Navigation

```toml
[modifiers]
primary = "ctrl"
secondary = "ctrl+shift"

[actions]
# Vim-style scrolling (Ctrl+D/U instead of Ctrl+J/K)
scroll_down = "ctrl+d"
scroll_up = "ctrl+u"
half_page_down = "ctrl+shift+d"
half_page_up = "ctrl+shift+u"
```

### Example: Emacs-Style Shortcuts

```toml
[modifiers]
primary = "ctrl"
secondary = "ctrl+alt"

[actions]
# Emacs-style navigation
scroll_down = "ctrl+n"
scroll_up = "ctrl+p"

# Emacs-style session management
new_session = "ctrl+x+ctrl+n"  # Chord: Ctrl+X then Ctrl+N
session_manager = "ctrl+x+b"   # Chord: Ctrl+X then B
```

### Example: Safety First

Remap dangerous actions to require more deliberate input:

```toml
[modifiers]
primary = "alt"
secondary = "alt+shift"

[actions]
# Require Ctrl+Shift+Q to quit (prevent accidental exits)
quit = "ctrl+shift+q"

# Require confirmation modifier for plugin force-shutdown
force_shutdown = "ctrl+alt+shift+f"
```

## Keybinding Syntax Rules

Understanding how to write keybinding strings correctly:

### Modifier Keys

Available modifiers (combine with `+`):
- `alt` - Alt/Option key
- `ctrl` - Control key
- `super` - Windows/Command key
- `shift` - Only for special keys (see below)

Examples: `alt+j`, `ctrl+shift+f`, `super+n`

### Letter Keys with Shift

**IMPORTANT:** When using Shift with letter keys, use **uppercase letters**, not `shift+`:

✅ **Correct:**
```toml
half_page_down = "alt+D"    # Alt+Shift+D
quit = "ctrl+Q"             # Ctrl+Shift+Q
```

❌ **Wrong:**
```toml
half_page_down = "alt+shift+d"  # Won't work!
quit = "ctrl+shift+q"           # Won't work!
```

**Why?** Terminals send uppercase letters when Shift is pressed, not `shift+letter`.

### Special Keys

For non-letter keys, you **can** use `shift+`:

✅ **These work:**
```toml
force_shutdown = "ctrl+shift+f1"    # Function keys
scroll_down = "alt+shift+down"      # Arrow keys
page_down = "ctrl+shift+pgdown"     # Page keys
```

### Key Names Reference

- Letters: `a-z` (lowercase without shift) or `A-Z` (uppercase with shift)
- Arrows: `up`, `down`, `left`, `right`
- Navigation: `pgup`, `pgdown`, `home`, `end`
- Function: `f1`-`f12`
- Special: `enter`, `tab`, `space`, `esc`, `backspace`, `delete`

## Complete Action Reference

### Main View - Modal Toggles

| Action | Default Key | Description |
|--------|-------------|-------------|
| `help` | `Alt+H` | Toggle help screen |
| `new_session` | `Alt+N` | Create new chat session |
| `session_manager` | `Alt+S` | Open session manager |
| `edit_session` | `Alt+E` | Edit current session |
| `model_selector` | `Alt+M` | Open model selector |
| `search_messages` | `Alt+F` | Search current session |
| `search_all_sessions` | `Alt+Shift+F` | Search all sessions (global) |
| `plugin_manager` | `Alt+P` | Open plugin manager |
| `about` | `Alt+Shift+A` | Show about screen |
| `settings` | `Alt+Shift+S` | Open settings |

### Main View - Scrolling

| Action | Default Key | Description |
|--------|-------------|-------------|
| `scroll_down` | `Alt+J` | Scroll down 1 line |
| `scroll_up` | `Alt+K` | Scroll up 1 line |
| `scroll_down_arrow` | `Alt+Down` | Scroll down 1 line (arrow key) |
| `scroll_up_arrow` | `Alt+Up` | Scroll up 1 line (arrow key) |
| `half_page_down` | `Alt+Shift+J` | Scroll down half page |
| `half_page_up` | `Alt+Shift+K` | Scroll up half page |
| `half_page_down_arrow` | `Alt+Shift+Down` | Scroll down half page (arrow) |
| `half_page_up_arrow` | `Alt+Shift+Up` | Scroll up half page (arrow) |
| `page_down` | `Alt+PgDn` | Scroll down full page |
| `page_up` | `Alt+PgUp` | Scroll up full page |
| `scroll_to_top` | `Alt+G` | Jump to top |
| `scroll_to_bottom` | `Alt+Shift+G` | Jump to bottom |

### Main View - Actions

| Action | Default Key | Description |
|--------|-------------|-------------|
| `quit` | `Alt+Q` | Quit OTUI |
| `yank_last_response` | `Alt+Y` | Copy last AI response |
| `yank_conversation` | `Alt+C` | Copy entire conversation |
| `external_editor` | `Alt+I` | Open external editor for prompt |

### Model Selector

**Normal Mode (browsing models):**

| Action | Default Key | Description |
|--------|-------------|-------------|
| `model_selector_down` | `J` | Navigate down |
| `model_selector_up` | `K` | Navigate up |
| `model_selector_down_arrow` | `Down` | Navigate down (arrow) |
| `model_selector_up_arrow` | `Up` | Navigate up (arrow) |

**Filter Mode (after pressing `/`):**

| Action | Default Key | Description |
|--------|-------------|-------------|
| `model_selector_down_filtered` | `Alt+J` | Navigate down |
| `model_selector_up_filtered` | `Alt+K` | Navigate up |
| `model_selector_down_arrow_filtered` | `Alt+Down` | Navigate down (arrow) |
| `model_selector_up_arrow_filtered` | `Alt+Up` | Navigate up (arrow) |

**Other Actions:**

| Action | Default Key | Description |
|--------|-------------|-------------|
| `model_selector_refresh` | `Alt+R` | Refresh model list |
| `close_model_selector` | `Alt+M` or `Esc` | Close modal |

### Plugin Manager

**Normal Mode (browsing plugins):**

| Action | Default Key | Description |
|--------|-------------|-------------|
| `plugin_down` | `J` | Navigate down |
| `plugin_up` | `K` | Navigate up |
| `plugin_down_arrow` | `Down` | Navigate down (arrow) |
| `plugin_up_arrow` | `Up` | Navigate up (arrow) |

**Filter Mode (after pressing `/`):**

| Action | Default Key | Description |
|--------|-------------|-------------|
| `plugin_down_filtered` | `Alt+J` | Navigate down |
| `plugin_up_filtered` | `Alt+K` | Navigate up |
| `plugin_down_arrow_filtered` | `Alt+Down` | Navigate down (arrow) |
| `plugin_up_arrow_filtered` | `Alt+Up` | Navigate up (arrow) |

**Note:** Press `Esc` to exit filter mode and clear the search input.

**Other Actions (Normal Mode):**

| Action | Default Key | Description |
|--------|-------------|-------------|
| `plugin_install` | `I` | Install plugin |
| `plugin_refresh` | `Alt+R` | Refresh plugin registry |

### Settings

| Action | Default Key | Description |
|--------|-------------|-------------|
| `settings_down` | `J` | Navigate down |
| `settings_up` | `K` | Navigate up |
| `settings_down_arrow` | `Down` | Navigate down (arrow) |
| `settings_up_arrow` | `Up` | Navigate up (arrow) |

### Provider Settings

**Note:** Accessed from within Settings (not from main view).

| Action | Default Key | Description |
|--------|-------------|-------------|
| `provider_down` | `J` | Navigate down |
| `provider_up` | `K` | Navigate up |
| `provider_down_arrow` | `Down` | Navigate down (arrow) |
| `provider_up_arrow` | `Up` | Navigate up (arrow) |

### About Modal

| Action | Default Key | Description |
|--------|-------------|-------------|
| `close_about` | `Alt+A` | Close about screen |

### Welcome Wizard

| Action | Default Key | Description |
|--------|-------------|-------------|
| `welcome_down` | `Alt+J` | Navigate down |
| `welcome_up` | `Alt+K` | Navigate up |
| `welcome_down_arrow` | `Alt+Down` | Navigate down (arrow) |
| `welcome_up_arrow` | `Alt+Up` | Navigate up (arrow) |
| `welcome_quit` | `Alt+Q` | Quit wizard |

## Universal Actions

These actions work in all text input contexts (Settings, Provider Settings, Session Manager, Plugin Manager configuration, Welcome Wizard, etc.):

| Action | Default Key | Description |
|--------|-------------|-------------|
| `clear_input` | `Alt+U` | Clear the currently focused input field |

## Examples

### Complete Vim-Style Configuration

```toml
[modifiers]
primary = "ctrl"
secondary = "ctrl+shift"

[actions]
# Vim navigation
scroll_down = "ctrl+j"
scroll_up = "ctrl+k"
half_page_down = "ctrl+d"
half_page_up = "ctrl+u"
scroll_to_top = "g+g"      # Double-tap G
scroll_to_bottom = "shift+g"

# Vim-style quit
quit = "shift+z+z"         # Like :wq in Vim

# Vim-style yank
yank_last_response = "y+y"
yank_conversation = "shift+y"
```

### macOS-Friendly Configuration

```toml
[modifiers]
primary = "cmd"          # Use Command key
secondary = "cmd+shift"

[actions]
# macOS-style shortcuts
new_session = "cmd+t"        # Like new tab in browsers
session_manager = "cmd+shift+o"
quit = "cmd+q"               # Standard macOS quit
```

### Minimal Conflict Configuration

Avoid conflicts with common tools:

```toml
[modifiers]
primary = "ctrl+alt"      # Rarely used by other tools
secondary = "ctrl+alt+shift"

# No per-action overrides needed - modifiers handle everything
```

## Troubleshooting

### My Custom Keybindings Don't Work

1. **Check TOML syntax:**
   ```bash
   cat ~/.local/share/otui/keybindings.toml
   ```
   Make sure there are no syntax errors (missing quotes, wrong brackets, etc.)

2. **Restart OTUI:**
   Keybindings are loaded on startup, not hot-reloaded.

3. **Check for conflicts:**
   If `Ctrl+J` doesn't work, your terminal or OS might be intercepting it.
   Try a different modifier or key combination.

### Help Screen Shows Wrong Keys

The help screen shows modifier-based keybindings. If you've customized individual actions, consult your `keybindings.toml` file for the actual mappings.

### OS/Terminal Intercepts My Keys

Some key combinations are reserved by the OS or terminal:

**macOS:**
- `Cmd+Q`, `Cmd+W`, `Cmd+Tab` - System shortcuts
- **Solution:** Use `ctrl` or `alt` modifiers instead

**Linux (i3/sway/etc.):**
- `Alt+[key]` - Window manager shortcuts
- **Solution:** Change `primary = "super"` or `primary = "ctrl"`

**Terminals:**
- `Ctrl+C` - SIGINT (cannot be rebound)
- `Ctrl+Z` - SIGTSTP (cannot be rebound)
- `Ctrl+\\` - SIGQUIT (cannot be rebound)
- **Solution:** Avoid these combinations

**Tmux:**
- `Ctrl+B` (default prefix) or `Alt` (if reconfigured)
- **Solution:** Use `super` or change tmux prefix

### Arrow Keys vs J/K

Both work! OTUI accepts both letter keys and arrow keys for navigation:
- `Alt+J` and `Alt+Down` both scroll down
- `Alt+K` and `Alt+Up` both scroll up

In modals, unmodified arrow keys also work (`Down`, `Up`).

### Keybindings File Not Created

Run OTUI once to auto-create the default file:
```bash
otui
```

Or create it manually:
```bash
mkdir -p ~/.local/share/otui
cat > ~/.local/share/otui/keybindings.toml <<'EOF'
[modifiers]
primary = "alt"
secondary = "alt+shift"
EOF
chmod 600 ~/.local/share/otui/keybindings.toml
```

### Reset to Defaults

Delete the file and restart OTUI:
```bash
rm ~/.local/share/otui/keybindings.toml
otui
```

OTUI will recreate the default configuration.

## See Also

- [OTUI Documentation](../README.md)
- [Configuration Guide](./CONFIGURATION.md)
- [Plugin System](./PLUGINS.md)

## Feedback

Found a keybinding conflict we should document? Have a useful configuration to share?

Open an issue: https://github.com/your-repo/otui/issues
