package mcp

import (
	"fmt"
	"regexp"
	"strings"

	globalconfig "otui/config"
)

// ArgPair represents a command-line argument for custom plugin definition
type ArgPair struct {
	Flag  string // e.g., "--dir", "-p", "-y"
	Type  string // "none", "value", "fixed"
	Value string // For type="fixed": the fixed value
	Label string // For type="value": label shown in configure screen
}

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

// ArgsToString converts []ArgPair to args string with value:: tags
func ArgsToString(args []ArgPair) string {
	parts := []string{}
	for _, arg := range args {
		switch arg.Type {
		case "none":
			parts = append(parts, arg.Flag)
		case "fixed":
			parts = append(parts, arg.Flag, arg.Value)
		case "value":
			parts = append(parts, arg.Flag, fmt.Sprintf("value::'%s'", arg.Label))
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

		// Check if it's a value:: tag
		if strings.HasPrefix(token, "value::") {
			// This shouldn't be a standalone token, skip
			continue
		}

		// Check if token looks like a flag
		if strings.HasPrefix(token, "-") {
			// Check if next token exists
			if i+1 < len(tokens) {
				nextToken := tokens[i+1]

				// Check if next token is value:: tag
				if strings.HasPrefix(nextToken, "value::'") && strings.HasSuffix(nextToken, "'") {
					label := strings.TrimSuffix(strings.TrimPrefix(nextToken, "value::'"), "'")
					args = append(args, ArgPair{
						Flag:  token,
						Type:  "value",
						Label: label,
					})
					i++ // Skip next token
					continue
				}

				// Check if next token is NOT a flag (fixed value)
				if !strings.HasPrefix(nextToken, "-") && !strings.HasPrefix(nextToken, "value::") {
					args = append(args, ArgPair{
						Flag:  token,
						Type:  "fixed",
						Value: nextToken,
					})
					i++ // Skip next token
					continue
				}
			}

			// Flag with no value
			args = append(args, ArgPair{
				Flag: token,
				Type: "none",
			})
		}
	}

	return args
}

// tokenizeArgs splits args string into tokens, handling value:: tags as single tokens
func tokenizeArgs(argsString string) []string {
	tokens := []string{}
	current := ""

	for i := 0; i < len(argsString); i++ {
		char := argsString[i]

		// Check for start of value:: tag
		if i+7 < len(argsString) && argsString[i:i+7] == "value::'" {
			// Save current token if exists
			if current != "" {
				tokens = append(tokens, current)
				current = ""
			}

			// Find closing quote
			endIdx := strings.Index(argsString[i+7:], "'")
			if endIdx != -1 {
				// Extract entire value:: tag
				valueTag := argsString[i : i+7+endIdx+1]
				tokens = append(tokens, valueTag)
				i = i + 7 + endIdx
				continue
			}
		}

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

	// 1. Add existing ConfigSchema (if any)
	schema = append(schema, plugin.ConfigSchema...)

	// 2. Parse Environment field
	if plugin.Environment != "" {
		envVars := strings.Fields(plugin.Environment)
		for _, envVar := range envVars {
			schema = append(schema, ConfigField{
				Key:      envVar,
				Label:    envVar,
				Type:     "string",
				Required: true,
			})
		}
	}

	// 3. Parse Args field for value:: tags
	if plugin.Args != "" {
		re := regexp.MustCompile(`value::'([^']+)'`)
		matches := re.FindAllStringSubmatch(plugin.Args, -1)

		for i, match := range matches {
			label := match[1]
			key := fmt.Sprintf("ARG_%d", i)

			schema = append(schema, ConfigField{
				Key:      key,
				Label:    label,
				Type:     "string",
				Required: true,
			})
		}
	}

	return schema
}

// SubstituteArgs replaces value::'Label' tags with actual user values
func SubstituteArgs(argsString string, userConfig map[string]string) []string {
	if argsString == "" {
		return []string{}
	}

	if globalconfig.DebugLog != nil {
		globalconfig.DebugLog.Printf("[ConfigBuilder] SubstituteArgs: argsString=%s, userConfig=%v", argsString, userConfig)
	}

	result := argsString
	re := regexp.MustCompile(`value::'([^']+)'`)
	matches := re.FindAllStringSubmatch(argsString, -1)

	for i, match := range matches {
		key := fmt.Sprintf("ARG_%d", i)
		value := userConfig[key]

		if globalconfig.DebugLog != nil {
			globalconfig.DebugLog.Printf("[ConfigBuilder] Replacing '%s' with '%s' (key=%s)", match[0], value, key)
		}

		result = strings.Replace(result, match[0], value, 1)
	}

	// Split into args array
	args := strings.Fields(result)

	if globalconfig.DebugLog != nil {
		globalconfig.DebugLog.Printf("[ConfigBuilder] Final args: %v", args)
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
