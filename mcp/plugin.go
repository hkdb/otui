package mcp

import "time"

type Plugin struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Category     string        `json:"category"`
	Tags         []string      `json:"tags,omitempty"`
	License      string        `json:"license,omitempty"`
	Repository   string        `json:"repository"`
	Author       string        `json:"author"`
	Stars        int           `json:"stars,omitempty"`
	Downloads    int           `json:"downloads,omitempty"`
	UpdatedAt    time.Time     `json:"updated_at,omitempty"`
	Language     string        `json:"language,omitempty"`
	InstallType  string        `json:"install_type"`
	Package      string        `json:"package,omitempty"`
	Command      string        `json:"command,omitempty"` // Custom command for manual/docker plugins
	RuntimeDeps  []string      `json:"runtime_deps,omitempty"`
	Verified     bool          `json:"verified"`
	Official     bool          `json:"official"`
	RequiresKey  bool          `json:"requires_key,omitempty"`
	Custom       bool          `json:"custom,omitempty"`
	ConfigSchema []ConfigField `json:"config_schema,omitempty"`
	Environment  string        `json:"environment,omitempty"`
	Args         string        `json:"args,omitempty"`
}

type ConfigField struct {
	Key          string
	Label        string
	Type         string
	Required     bool
	DefaultValue string
	Description  string
}
