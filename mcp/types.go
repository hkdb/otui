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
}

type PluginConfig struct {
	ID         string
	Runtime    string
	EntryPoint string
	Args       []string
	Env        map[string]string
	Config     map[string]string
}
