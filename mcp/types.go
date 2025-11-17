package mcp

import (
	"os/exec"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

type PluginProcess struct {
	ID        string
	Name      string
	Command   string
	Args      []string
	Process   *exec.Cmd
	Client    *client.Client
	Transport transport.Interface
	Tools     []mcptypes.Tool
	Running   bool
	Error     error
	IsRemote  bool   // Remote plugins don't have local processes
	ServerURL string // URL for remote plugins
}

type PluginConfig struct {
	ID         string
	Runtime    string
	EntryPoint string
	Args       []string
	Env        map[string]string
	Config     map[string]string
	ServerURL  string // For remote plugins
	AuthType   string // "none", "headers", "oauth"
	Transport  string // "sse" (default), "streamable-http"
}
