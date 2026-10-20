package mcp

import (
	"fmt"
	"regexp"
	"strings"

	globalconfig "otui/config"
)

// ArgPair represents a command-line argument for custom plugin definition
type ArgPair struct {
	Flag    string // e.g., "--dir", "-p", "-y" ("" for a positional argument)
	Type    string // "none", "value", "fixed"
	Value   string // For type="fixed": the fixed value
	Label   string // For type="value": label shown in configure screen
	Default string // For type="value": optional default (value::'Label';;'default')
}

// valueTagPrefix starts an argument template: value::'Label' with an
// optional default: value::'Label';;'default'
const valueTagPrefix = "value::'"

// valueTagRe matches a value tag and its optional default
var valueTagRe = regexp.MustCompile(`value::'([^']+)'(?:;;'([^']*)')?`)

// EnvVarsToString converts environment variable keys to space-separated string
func EnvVarsToString(envVarKeys []string) string {
	return strings.Join(envVarKeys, " ")
}

// StringToEnvVars converts space-separated string to env var keys
func StringToEnvVars(envString string) []string {
	if envString == "" {
		return []string{}
	}
	return strings.Fields(envString)
}

// EnvironmentKeys returns the env var keys of a plugin's Environment field
// (without their ;;'default' suffixes)
func EnvironmentKeys(envString string) []string {
	defs := parseEnvironmentWithDefaults(envString)
	keys := make([]string, len(defs))
	for i, def := range defs {
		keys[i] = def.Key
	}
	return keys
}

// ArgsToString converts []ArgPair to args string with value:: tags
func ArgsToString(args []ArgPair) string {
	parts := []string{}
	for _, arg := range args {
		if arg.Flag != "" {
			parts = append(parts, arg.Flag)
		}
		switch arg.Type {
		case "fixed":
			parts = append(parts, arg.Value)
		case "value":
			tag := fmt.Sprintf("value::'%s'", arg.Label)
			if arg.Default != "" {
				tag += fmt.Sprintf(";;'%s'", arg.Default)
			}
			parts = append(parts, tag)
		}
	}
	return strings.Join(parts, " ")
}

// StringToArgs parses args string to []ArgPair
func StringToArgs(argsString string) []ArgPair {
	if argsString == "" {
		return []ArgPair{}
	}

	args := []ArgPair{}
	tokens := tokenizeArgs(argsString)

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]

		// Positional value tag
		if label, def, ok := parseValueTag(token); ok {
			args = append(args, ArgPair{Type: "value", Label: label, Default: def})
			continue
		}

		// Positional fixed value
		if !strings.HasPrefix(token, "-") {
			args = append(args, ArgPair{Type: "fixed", Value: token})
			continue
		}

		if i+1 < len(tokens) {
			nextToken := tokens[i+1]

			// Flag followed by a value tag
			if label, def, ok := parseValueTag(nextToken); ok {
				args = append(args, ArgPair{Flag: token, Type: "value", Label: label, Default: def})
				i++ // Skip next token
				continue
			}

			// Flag followed by a fixed value
			if !strings.HasPrefix(nextToken, "-") {
				args = append(args, ArgPair{Flag: token, Type: "fixed", Value: nextToken})
				i++ // Skip next token
				continue
			}
		}

		// Flag with no value
		args = append(args, ArgPair{Flag: token, Type: "none"})
	}

	return args
}

// parseValueTag parses a token that is exactly value::'Label' or
// value::'Label';;'default'
func parseValueTag(token string) (label, def string, ok bool) {
	m := valueTagRe.FindStringSubmatchIndex(token)
	if m == nil || m[0] != 0 || m[1] != len(token) {
		return "", "", false
	}
	label = token[m[2]:m[3]]
	if m[4] >= 0 {
		def = token[m[4]:m[5]]
	}
	return label, def, true
}

// tokenizeArgs splits args string on spaces, keeping value:: tags (whose
// labels and defaults may contain spaces) intact
func tokenizeArgs(argsString string) []string {
	tokens := []string{}
	current := ""

	for i := 0; i < len(argsString); i++ {
		// A value tag (and its optional default) is part of the current token
		if strings.HasPrefix(argsString[i:], valueTagPrefix) {
			if m := valueTagRe.FindStringIndex(argsString[i:]); m != nil && m[0] == 0 {
				current += argsString[i : i+m[1]]
				i += m[1] - 1
				continue
			}
		}

		char := argsString[i]
		if char == ' ' {
			if current != "" {
				tokens = append(tokens, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}

	if current != "" {
		tokens = append(tokens, current)
	}

	return tokens
}

// BuildFullConfigSchema generates ConfigSchema from plugin's ConfigSchema, Environment, and Args
func BuildFullConfigSchema(plugin *Plugin) []ConfigField {
	schema := []ConfigField{}

	// 1. For remote plugins, add server_url as first field
	if plugin.InstallType == "remote" && plugin.ServerURL != "" {
		schema = append(schema, ConfigField{
			Key:          "server_url",
			Label:        "Server URL",
			Type:         "string",
			Required:     false,
			DefaultValue: plugin.ServerURL,
			Description:  "Override the default server URL for this plugin",
		})
	}

	// 2. Add existing ConfigSchema (if any)
	schema = append(schema, plugin.ConfigSchema...)

	// 3. Parse Environment field with optional defaults
	if plugin.Environment != "" {
		envVars := parseEnvironmentWithDefaults(plugin.Environment)
		for _, envVar := range envVars {
			schema = append(schema, ConfigField{
				Key:          envVar.Key,
				Label:        envVar.Key,
				Type:         "string",
				Required:     envVar.Required,
				DefaultValue: envVar.DefaultValue,
			})
		}
	}

	// 4. Parse Args field with optional defaults
	if plugin.Args != "" {
		argDefs := parseArgsWithDefaults(plugin.Args)
		for i, argDef := range argDefs {
			key := fmt.Sprintf("ARG_%d", i)
			schema = append(schema, ConfigField{
				Key:          key,
				Label:        argDef.Label,
				Type:         "string",
				Required:     argDef.Required,
				DefaultValue: argDef.DefaultValue,
			})
		}
	}

	return schema
}

// SubstituteArgs replaces value::'Label' tags with the user's values (config
// keys ARG_0, ARG_1, ... in tag order) or their defaults. Each substituted
// value is one argument, even if it contains spaces. A tag without a value is
// dropped together with the flag directly before it.
func SubstituteArgs(argsString string, userConfig map[string]string) []string {
	if argsString == "" {
		return []string{}
	}

	args := []string{}
	argIndex := 0
	prevWasFlag := false

	for _, token := range tokenizeArgs(argsString) {
		if !strings.Contains(token, valueTagPrefix) {
			args = append(args, token)
			prevWasFlag = strings.HasPrefix(token, "-")
			continue
		}

		_, _, tagOnly := parseValueTag(token)
		arg := valueTagRe.ReplaceAllStringFunc(token, func(tag string) string {
			m := valueTagRe.FindStringSubmatch(tag)
			value := userConfig[fmt.Sprintf("ARG_%d", argIndex)]
			argIndex++
			if value == "" {
				value = m[2]
			}
			return value
		})

		if tagOnly && arg == "" {
			// No value: drop the flag it belongs to as well
			if prevWasFlag {
				args = args[:len(args)-1]
			}
			prevWasFlag = false
			continue
		}

		args = append(args, arg)
		prevWasFlag = false
	}

	if globalconfig.DebugLog != nil {
		globalconfig.DebugLog.Printf("[ConfigBuilder] SubstituteArgs: %d args", len(args))
	}

	return args
}

// BuildEnvMap extracts environment variables from plugin.Environment and user config
func BuildEnvMap(envString string, userConfig map[string]string) map[string]string {
	envMap := make(map[string]string)

	if envString != "" {
		envVars := strings.Fields(envString)
		for _, envVar := range envVars {
			if val, ok := userConfig[envVar]; ok {
				envMap[envVar] = val
			}
		}
	}

	return envMap
}

// ArgDefinition represents a parsed argument with optional default value
type ArgDefinition struct {
	Label        string
	DefaultValue string
	Required     bool
}

// parseArgsWithDefaults extracts arg definitions with optional defaults
// Format: "value::'Label';;'default'"
// Examples:
//
//	"value::'Directory'" → {Label: "Directory", DefaultValue: "", Required: true}
//	"value::'Directory';;'./'" → {Label: "Directory", DefaultValue: "./", Required: false}
func parseArgsWithDefaults(argsString string) []ArgDefinition {
	result := []ArgDefinition{}

	// Regex to match: value::'Label' or value::'Label';;'default'
	// Group 1: Label (required)
	// Group 2: Optional default (after ;;)
	matches := valueTagRe.FindAllStringSubmatch(argsString, -1)

	for _, match := range matches {
		label := match[1]
		defaultValue := ""
		required := true

		// Check if default value exists (group 2)
		if len(match) > 2 && match[2] != "" {
			defaultValue = match[2]
			required = false // Has default, not required
		}

		result = append(result, ArgDefinition{
			Label:        label,
			DefaultValue: defaultValue,
			Required:     required,
		})
	}

	return result
}

// EnvVarDefinition represents a parsed environment variable with optional default value
type EnvVarDefinition struct {
	Key          string
	DefaultValue string
	Required     bool
}

// parseEnvironmentWithDefaults parses environment string with optional defaults
// Format: "KEY1 KEY2;;'default2' KEY3;;'default3'"
// Examples:
//
//	"API_KEY" → {Key: "API_KEY", DefaultValue: "", Required: true}
//	"session-id;;'{{OTUI_SESSION_ID}}'" → {Key: "session-id", DefaultValue: "{{OTUI_SESSION_ID}}", Required: false}
func parseEnvironmentWithDefaults(envString string) []EnvVarDefinition {
	result := []EnvVarDefinition{}

	// Split by spaces (handles both formats)
	tokens := strings.Fields(envString)

	for _, token := range tokens {
		// Handle with delimiter first
		if strings.Contains(token, ";;") {
			parts := strings.SplitN(token, ";;", 2)
			key := parts[0]

			// Validate: default must be wrapped in quotes
			if len(parts) > 1 && strings.HasPrefix(parts[1], "'") && strings.HasSuffix(parts[1], "'") {
				// Valid format: key;;'default'
				defaultValue := strings.Trim(parts[1], "'")
				result = append(result, EnvVarDefinition{
					Key:          key,
					DefaultValue: defaultValue,
					Required:     false,
				})
				continue
			}

			// Malformed default (missing quotes) - treat as required
			if globalconfig.DebugLog != nil {
				globalconfig.DebugLog.Printf("[ConfigBuilder] Invalid default format for '%s': missing quotes around '%s'", key, parts[1])
			}
			result = append(result, EnvVarDefinition{
				Key:          key,
				DefaultValue: "",
				Required:     true,
			})
			continue
		}

		// No delimiter - required field (no else needed)
		result = append(result, EnvVarDefinition{
			Key:          token,
			DefaultValue: "",
			Required:     true,
		})
	}

	return result
}

// sanitizeDataDirToUser converts filesystem path to sanitized user identifier
// Example: /home/alice/otui → home-alice-otui
func sanitizeDataDirToUser(dataDir string) string {
	// Remove leading/trailing slashes
	clean := strings.Trim(dataDir, "/")

	// Replace path separators with hyphens
	sanitized := strings.ReplaceAll(clean, "/", "-")

	// Replace any remaining problematic characters with hyphens
	sanitized = strings.ReplaceAll(sanitized, " ", "-")
	sanitized = strings.ReplaceAll(sanitized, "\\", "-")

	return sanitized
}

// SubstituteSessionVars replaces template variables in environment values with session context
// Supported templates:
//
//	{{OTUI_SESSION_ID}}   - Unique conversation UUID (e.g., "abc-123-def")
//	{{OTUI_SESSION_NAME}} - Human-readable session name (e.g., "Work Project")
//	{{OTUI_DATA_DIR}}     - Raw filesystem path (e.g., "/home/alice/otui")
//	{{OTUI_USER}}         - Sanitized data dir identifier (e.g., "home-alice-otui")
func SubstituteSessionVars(env map[string]string, sessionID, sessionName, dataDir string) map[string]string {
	if len(env) == 0 {
		return env
	}

	// Build replacement map
	replacements := map[string]string{
		"{{OTUI_SESSION_ID}}":   sessionID,
		"{{OTUI_SESSION_NAME}}": sessionName,
		"{{OTUI_DATA_DIR}}":     dataDir,
		"{{OTUI_USER}}":         sanitizeDataDirToUser(dataDir),
	}

	// Create new map with substituted values
	result := make(map[string]string, len(env))
	for k, v := range env {
		substituted := v
		for template, value := range replacements {
			substituted = strings.ReplaceAll(substituted, template, value)
		}
		result[k] = substituted

		// Debug log if substitution occurred
		if globalconfig.DebugLog != nil && substituted != v {
			globalconfig.DebugLog.Printf("[ConfigBuilder] SubstituteSessionVars: %s = '%s' → '%s'", k, globalconfig.Redact(k, v), globalconfig.Redact(k, substituted))
		}
	}

	return result
}
