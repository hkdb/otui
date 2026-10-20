package config

import (
	"regexp"
	"sort"
	"strings"
)

// secretNameRe matches names (env vars, config keys, field labels) whose
// values are secrets
var secretNameRe = regexp.MustCompile(`(?i)key|token|secret|passw|passphrase|credential|auth|cookie|session`)

// Redact returns value for logging: "****" if name looks like it holds a
// secret (API key, token, password...), otherwise value
func Redact(name, value string) string {
	if value != "" && secretNameRe.MatchString(name) {
		return "****"
	}
	return value
}

// RedactMap returns a loggable copy of a config map (see Redact)
func RedactMap(m map[string]string) map[string]string {
	redacted := make(map[string]string, len(m))
	for k, v := range m {
		redacted[k] = Redact(k, v)
	}
	return redacted
}

// EnvNames returns the variable names of an environment (KEY=value list),
// sorted, for logging without values
func EnvNames(env []string) []string {
	names := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
