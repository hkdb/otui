package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"otui/config"
)

const registryURL = "https://raw.githubusercontent.com/hkdb/otui-registry/main/plugins.json"

type Registry struct {
	plugins       []Plugin
	customPlugins []Plugin
	cacheDir      string
}

type GitHubRepoInfo struct {
	StargazersCount int    `json:"stargazers_count"`
	Language        string `json:"language"`
	License         *struct {
		SPDXID string `json:"spdx_id"`
	} `json:"license"`
	PushedAt    time.Time `json:"pushed_at"`
	Description string    `json:"description"`
	FullName    string    `json:"full_name"`
}

type GitHubTreeItem struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type GitHubTree struct {
	Tree []GitHubTreeItem `json:"tree"`
}

func NewRegistry(dataDir string) (*Registry, error) {
	cacheDir := filepath.Join(dataDir, "registry")
	if err := config.EnsureDir(cacheDir); err != nil {
		return nil, fmt.Errorf("failed to create registry cache dir: %w", err)
	}

	r := &Registry{
		cacheDir: cacheDir,
	}

	if err := r.Load(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Registry) Load() error {
	cachedPath := filepath.Join(r.cacheDir, "plugin_registry.json")

	if config.FileExists(cachedPath) {
		data, err := os.ReadFile(cachedPath)
		if err == nil {
			var cached []Plugin
			if err := json.Unmarshal(data, &cached); err == nil {
				r.plugins = cached
			}
		}
	}

	if len(r.plugins) == 0 {
		r.plugins = []Plugin{}
	}

	customPath := filepath.Join(r.cacheDir, "custom_plugins.json")
	if config.FileExists(customPath) {
		data, err := os.ReadFile(customPath)
		if err == nil {
			if err := json.Unmarshal(data, &r.customPlugins); err == nil {
				for i := range r.customPlugins {
					r.customPlugins[i].Custom = true
				}
			}
		}
	}

	return nil
}

func (r *Registry) Refresh() error {
	// Don't preserve custom plugins from r.plugins - they're managed separately in r.customPlugins
	// GetAll() combines both arrays, so we only need to refresh the official registry here

	freshPlugins, err := fetchRegistry()
	if err != nil {
		return fmt.Errorf("failed to fetch plugin registry: %w", err)
	}

	r.plugins = freshPlugins

	cachedPath := filepath.Join(r.cacheDir, "plugin_registry.json")
	data, err := json.MarshalIndent(freshPlugins, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal plugins: %w", err)
	}

	if err := os.WriteFile(cachedPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write cache: %w", err)
	}

	return nil
}

func (r *Registry) GetAll() []Plugin {
	all := make([]Plugin, 0, len(r.plugins)+len(r.customPlugins))
	all = append(all, r.plugins...)
	all = append(all, r.customPlugins...)
	return all
}

func (r *Registry) Search(query string) []Plugin {
	query = strings.ToLower(query)
	var results []Plugin

	for _, p := range r.GetAll() {
		if strings.Contains(strings.ToLower(p.Name), query) ||
			strings.Contains(strings.ToLower(p.Description), query) ||
			strings.Contains(strings.ToLower(p.Category), query) ||
			containsTag(p.Tags, query) {
			results = append(results, p)
		}
	}

	return results
}

func (r *Registry) GetByID(id string) *Plugin {
	for _, p := range r.GetAll() {
		if p.ID == id {
			return &p
		}
	}
	return nil
}

func (r *Registry) AddCustomPlugin(repoURL string) (*Plugin, []string, error) {
	if err := validateGitHubURL(repoURL); err != nil {
		return nil, nil, fmt.Errorf("invalid GitHub URL: %w", err)
	}

	repoPath := strings.TrimPrefix(repoURL, "https://github.com/")
	repoPath = strings.TrimSuffix(repoPath, "/")

	ghInfo, err := FetchGitHubRepoInfo(repoPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch repository info: %w", err)
	}

	isMCP, warnings := verifyMCPServer(repoPath)

	trustWarnings := []string{}
	if !isMCP {
		trustWarnings = append(trustWarnings, "⚠️  Cannot verify this is an MCP server")
	}
	if ghInfo.StargazersCount < 10 {
		trustWarnings = append(trustWarnings, fmt.Sprintf("⚠️  Low stars: %d", ghInfo.StargazersCount))
	}
	if ghInfo.License == nil || ghInfo.License.SPDXID == "NOASSERTION" {
		trustWarnings = append(trustWarnings, "⚠️  No license detected")
	}
	if time.Since(ghInfo.PushedAt) > 365*24*time.Hour {
		trustWarnings = append(trustWarnings, fmt.Sprintf("⚠️  Last commit over 1 year ago (%s)", ghInfo.PushedAt.Format("2006-01-02")))
	}
	trustWarnings = append(trustWarnings, warnings...)

	parts := strings.Split(repoPath, "/")
	author := parts[0]
	repoName := parts[len(parts)-1]

	id := generatePluginID(repoName)

	installType := "manual"
	pkg := ""

	license := ""
	if ghInfo.License != nil {
		license = ghInfo.License.SPDXID
	}

	plugin := &Plugin{
		ID:          id,
		Name:        ghInfo.FullName,
		Description: ghInfo.Description,
		Category:    "custom",
		Repository:  repoURL,
		Author:      author,
		Stars:       ghInfo.StargazersCount,
		Language:    ghInfo.Language,
		License:     license,
		UpdatedAt:   ghInfo.PushedAt,
		InstallType: installType,
		Package:     pkg,
		Verified:    false,
		Official:    false,
		Custom:      true,
	}

	r.customPlugins = append(r.customPlugins, *plugin)

	if err := r.saveCustomPlugins(); err != nil {
		return nil, nil, fmt.Errorf("failed to save custom plugins: %w", err)
	}

	return plugin, trustWarnings, nil
}

// AddCustomPluginDirect adds a pre-built plugin to custom plugins
func (r *Registry) AddCustomPluginDirect(plugin *Plugin) ([]string, error) {
	if plugin == nil {
		return nil, fmt.Errorf("plugin cannot be nil")
	}

	// Ensure it's marked as custom
	plugin.Custom = true

	// Check for duplicate ID
	for _, p := range r.customPlugins {
		if p.ID == plugin.ID {
			return nil, fmt.Errorf("plugin with ID %s already exists", plugin.ID)
		}
	}

	// Generate trust warnings (basic check)
	trustWarnings := []string{}
	if !plugin.Verified {
		trustWarnings = append(trustWarnings, "⚠️  This plugin is not verified by the OTUI team")
	}
	if plugin.Repository == "" {
		trustWarnings = append(trustWarnings, "⚠️  No repository URL provided")
	}

	// Add to custom plugins list only (GetAll() will combine with r.plugins)
	r.customPlugins = append(r.customPlugins, *plugin)

	if err := r.saveCustomPlugins(); err != nil {
		return nil, fmt.Errorf("failed to save custom plugins: %w", err)
	}

	return trustWarnings, nil
}

func (r *Registry) RemoveCustomPlugin(id string) error {
	found := false

	// Remove from r.customPlugins
	for i, p := range r.customPlugins {
		if p.ID == id {
			r.customPlugins = append(r.customPlugins[:i], r.customPlugins[i+1:]...)
			found = true
			break
		}
	}

	// Also remove from r.plugins (cleanup for old duplicates that may exist)
	for i, p := range r.plugins {
		if p.ID == id && p.Custom {
			r.plugins = append(r.plugins[:i], r.plugins[i+1:]...)
			break
		}
	}

	if !found {
		return fmt.Errorf("plugin not found")
	}

	return r.saveCustomPlugins()
}

func (r *Registry) saveCustomPlugins() error {
	customPath := filepath.Join(r.cacheDir, "custom_plugins.json")
	data, err := json.MarshalIndent(r.customPlugins, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(customPath, data, 0600)
}

func fetchRegistry() ([]Plugin, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(registryURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var plugins []Plugin
	if err := json.NewDecoder(resp.Body).Decode(&plugins); err != nil {
		return nil, fmt.Errorf("failed to parse registry JSON: %w", err)
	}

	return plugins, nil
}

func generatePluginID(name string) string {
	id := strings.ToLower(name)
	id = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(id, "-")
	id = strings.Trim(id, "-")
	return id
}

func extractAuthor(repo string) string {
	parts := strings.Split(strings.TrimPrefix(repo, "https://github.com/"), "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func validateGitHubURL(url string) error {
	if !strings.HasPrefix(url, "https://github.com/") {
		return fmt.Errorf("must be a GitHub URL (https://github.com/...)")
	}

	shellMetachars := []string{";", "&", "|", "$", "`", "(", ")", "<", ">", "\n", "\r"}
	for _, char := range shellMetachars {
		if strings.Contains(url, char) {
			return fmt.Errorf("URL contains invalid characters")
		}
	}

	parts := strings.Split(strings.TrimPrefix(url, "https://github.com/"), "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid GitHub repository URL format")
	}

	return nil
}

// FetchGitHubRepoInfo fetches repository information from GitHub API (exported for UI)
func FetchGitHubRepoInfo(repoPath string) (*GitHubRepoInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s", repoPath)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("repository not found (404)")
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var info GitHubRepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return &info, nil
}

func verifyMCPServer(repoPath string) (bool, []string) {
	warnings := []string{}

	url := fmt.Sprintf("https://api.github.com/repos/%s/git/trees/main?recursive=1", repoPath)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		warnings = append(warnings, "⚠️  Failed to verify repository structure")
		return false, warnings
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		warnings = append(warnings, "⚠️  Failed to verify repository structure")
		return false, warnings
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		warnings = append(warnings, "⚠️  Could not access repository files")
		return false, warnings
	}

	var tree GitHubTree
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		warnings = append(warnings, "⚠️  Failed to parse repository structure")
		return false, warnings
	}

	hasPackageJSON := false
	hasSetupPy := false
	hasMCPReference := false

	for _, item := range tree.Tree {
		if item.Type == "blob" {
			if item.Path == "package.json" {
				hasPackageJSON = true
			}
			if item.Path == "setup.py" || item.Path == "pyproject.toml" {
				hasSetupPy = true
			}
			if strings.Contains(strings.ToLower(item.Path), "mcp") {
				hasMCPReference = true
			}
		}
	}

	isMCP := (hasPackageJSON || hasSetupPy) && hasMCPReference

	if !isMCP {
		if !hasPackageJSON && !hasSetupPy {
			warnings = append(warnings, "⚠️  No package.json or setup.py found")
		}
		if !hasMCPReference {
			warnings = append(warnings, "⚠️  No MCP-related files detected")
		}
	}

	return isMCP, warnings
}

func containsTag(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}
