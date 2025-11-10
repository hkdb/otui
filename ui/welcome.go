package ui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"otui/config"
	"otui/ollama"
	"otui/provider"
)

type wizardStep int

const (
	stepWelcome wizardStep = iota
	stepSecurityMethod
	stepSSHKeySource
	stepCreateOTUIKey
	stepSetPassphrase
	stepSelectExistingKey
	stepVerifyExistingKeyPassphrase
	stepBackupReminder
	stepProviderSelection
	stepConfigureProvider
	stepOllamaURL
	stepModelSelection
	stepDataDirectory
	stepComplete
)

type WelcomeModel struct {
	step           wizardStep
	selectedButton int

	// Track if this is a restart scenario (creating new data dir from Settings)
	isRestartScenario bool

	// Security configuration
	securityMethod        config.SecurityMethod
	sshKeySource          int // 0 = create OTUI, 1 = use existing
	usePassphrase         bool
	sshKeyPath            string
	availableKeys         []string
	selectedKeyIndex      int
	passphraseInput       textinput.Model
	passphraseConfirm     textinput.Model
	activePassphraseField int             // 0 = passphrase, 1 = confirm
	existingKeyPassphrase textinput.Model // For validating existing key passphrase

	// Provider configuration
	providers          []config.ProviderConfig
	providerCheckboxes []bool // Which providers are enabled
	currentProviderIdx int    // Which provider we're configuring
	apiKeyInput        textinput.Model
	providerApiKeys    map[string]string // Track API keys collected during wizard

	// Multi-provider model aggregation
	providersPingStatus map[string]bool               // Which providers pinged successfully
	providersModels     map[string][]ollama.ModelInfo // Models per provider
	allModels           []ollama.ModelInfo            // Aggregated models for unified selection

	// Model filtering (wizard model selection)
	modelFilterMode      bool
	modelFilterInput     textinput.Model
	filteredWizardModels []ollama.ModelInfo

	// Ollama configuration (now just another provider)
	ollamaHost      string
	defaultModel    string
	defaultProvider string // Track which provider the selected model belongs to
	dataDirectory   string

	urlInput      textinput.Model
	dirInput      textinput.Model
	models        []string
	selectedModel int

	// Existing profile directory picker
	existingProfilePicker FilePickerState

	width  int
	height int

	err     string
	loading bool
}

var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true)

	featureStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	buttonStyle = lipgloss.NewStyle().
			Width(24).
			Align(lipgloss.Center).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8"))

	selectedButtonStyle = lipgloss.NewStyle().
				Width(24).
				Align(lipgloss.Center).
				Padding(0, 2).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("10")).
				Foreground(lipgloss.Color("10")).
				Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true)

	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("6")).
			Padding(0, 1)
)

func NewWelcomeModel() WelcomeModel {
	urlInput := textinput.New()
	urlInput.Placeholder = "http://localhost:11434"
	urlInput.Width = 50
	urlInput.CharLimit = 200

	dirInput := textinput.New()
	dirInput.Placeholder = "~/.local/share/otui"
	dirInput.Width = 50
	dirInput.CharLimit = 200

	apiKeyInput := textinput.New()
	apiKeyInput.Placeholder = "sk-ant-..."
	apiKeyInput.Width = 50
	apiKeyInput.CharLimit = 300
	apiKeyInput.EchoMode = textinput.EchoPassword
	apiKeyInput.EchoCharacter = '•'

	passphraseInput := textinput.New()
	passphraseInput.Placeholder = "Enter passphrase"
	passphraseInput.Width = 50
	passphraseInput.CharLimit = 200
	passphraseInput.EchoMode = textinput.EchoPassword
	passphraseInput.EchoCharacter = '•'

	passphraseConfirm := textinput.New()
	passphraseConfirm.Placeholder = "Confirm passphrase"
	passphraseConfirm.Width = 50
	passphraseConfirm.CharLimit = 200
	passphraseConfirm.EchoMode = textinput.EchoPassword
	passphraseConfirm.EchoCharacter = '•'

	existingKeyPassphrase := NewPassphraseInput("Enter passphrase for this SSH key")

	modelFilterInput := textinput.New()
	modelFilterInput.Placeholder = "Type to filter models..."
	modelFilterInput.Width = 50
	modelFilterInput.CharLimit = 100

	// Initialize existing profile picker
	existingProfilePicker := NewFilePickerState(FilePickerConfig{
		Title:          "Select Existing Profile Directory",
		Mode:           FilePickerModeOpen,
		AllowedTypes:   nil, // nil = allow all file types
		StartDirectory: config.GetHomeDir(),
		ShowHidden:     true,
		OperationType:  "Load Profile",
	})

	// Initialize available providers
	providers := []config.ProviderConfig{
		{ID: "ollama", Name: "Ollama (Local)", Enabled: false, BaseURL: "http://localhost:11434"},
		{ID: "openrouter", Name: "OpenRouter", Enabled: false, BaseURL: "https://openrouter.ai/api/v1"},
		{ID: "openai", Name: "OpenAI", Enabled: false, BaseURL: "https://api.openai.com/v1"},
		{ID: "anthropic", Name: "Anthropic", Enabled: false, BaseURL: "https://api.anthropic.com"},
	}

	// Detect restart scenario (system config exists but user config doesn't)
	isRestart := false
	initialDataDir := "~/.local/share/otui"
	if config.SystemConfigExists() {
		if systemCfg, err := config.LoadSystemConfig(); err == nil {
			dataDir := config.ExpandPath(systemCfg.DataDirectory)
			userConfigPath := filepath.Join(dataDir, "config.toml")
			if !config.FileExists(userConfigPath) {
				isRestart = true
				// Pre-populate data directory with user's choice from settings
				initialDataDir = systemCfg.DataDirectory
				dirInput.SetValue(initialDataDir)
			}
		}
	}

	return WelcomeModel{
		isRestartScenario:     isRestart,
		step:                  stepWelcome,
		selectedButton:        0,
		securityMethod:        config.SecurityPlainText,
		sshKeySource:          0,
		usePassphrase:         false,
		providers:             providers,
		providerCheckboxes:    make([]bool, len(providers)),
		providerApiKeys:       make(map[string]string),
		providersPingStatus:   make(map[string]bool),
		providersModels:       make(map[string][]ollama.ModelInfo),
		allModels:             []ollama.ModelInfo{},
		modelFilterInput:      modelFilterInput,
		filteredWizardModels:  []ollama.ModelInfo{},
		currentProviderIdx:    0,
		ollamaHost:            "http://localhost:11434",
		defaultModel:          "llama3.2:latest",
		dataDirectory:         initialDataDir,
		urlInput:              urlInput,
		dirInput:              dirInput,
		apiKeyInput:           apiKeyInput,
		passphraseInput:       passphraseInput,
		passphraseConfirm:     passphraseConfirm,
		existingKeyPassphrase: existingKeyPassphrase,
		activePassphraseField: 0,
		models:                []string{},
		selectedModel:         0,
		existingProfilePicker: existingProfilePicker,
	}
}

// fetchAllProviderModels returns batch command to fetch models from all configured providers
func (m *WelcomeModel) fetchAllProviderModels() tea.Cmd {
	var cmds []tea.Cmd

	for i, prov := range m.providers {
		if !m.providerCheckboxes[i] {
			continue
		}

		switch prov.ID {
		case "ollama":
			cmds = append(cmds, provider.FetchSingleProviderModels(
				"ollama",
				"",
				"",
				m.ollamaHost,
			))
		default:
			apiKey := m.providerApiKeys[prov.ID]
			cmds = append(cmds, provider.FetchSingleProviderModels(
				prov.ID,
				prov.BaseURL,
				apiKey,
				"",
			))
		}
	}

	return tea.Batch(cmds...)
}

func (m WelcomeModel) Init() tea.Cmd {
	return nil
}

type modelsLoadedMsg struct {
	models []string
	err    error
}

type urlValidatedMsg struct {
	valid bool
	err   error
}

func validateOllamaURL(url string) tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(url + "/api/version")
		if err != nil {
			return urlValidatedMsg{valid: false, err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return urlValidatedMsg{valid: false, err: fmt.Errorf("server returned status %d", resp.StatusCode)}
		}

		return urlValidatedMsg{valid: true, err: nil}
	}
}

func loadModels(url string) tea.Cmd {
	return func() tea.Msg {
		client, err := ollama.NewClient(url, "")
		if err != nil {
			return modelsLoadedMsg{models: nil, err: err}
		}

		ctx := context.Background()
		modelInfos, err := client.ListModels(ctx)
		if err != nil {
			return modelsLoadedMsg{models: nil, err: err}
		}

		models := make([]string, len(modelInfos))
		for i, info := range modelInfos {
			models[i] = info.Name
		}

		return modelsLoadedMsg{models: models, err: nil}
	}
}

func (m WelcomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Handle existing profile picker updates (non-KeyMsg like readDirMsg)
	if m.existingProfilePicker.Active && msg != nil {
		switch msg.(type) {
		case tea.KeyMsg:
			// KeyMsg handled in updateExistingProfilePicker below
		default:
			// Forward non-KeyMsg to picker
			m.existingProfilePicker.Picker, cmd = m.existingProfilePicker.Picker.Update(msg)
			return m, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Handle existing profile picker KeyMsg
		if m.existingProfilePicker.Active {
			return m.updateExistingProfilePicker(msg)
		}

		switch m.step {
		case stepWelcome:
			return m.updateWelcomeScreen(msg)
		case stepSecurityMethod:
			return m.updateSecurityMethodScreen(msg)
		case stepSSHKeySource:
			return m.updateSSHKeySourceScreen(msg)
		case stepCreateOTUIKey:
			return m.updateCreateOTUIKeyScreen(msg)
		case stepSetPassphrase:
			return m.updateSetPassphraseScreen(msg)
		case stepSelectExistingKey:
			return m.updateSelectExistingKeyScreen(msg)
		case stepVerifyExistingKeyPassphrase:
			return m.updateVerifyExistingKeyPassphraseScreen(msg)
		case stepBackupReminder:
			return m.updateBackupReminderScreen(msg)
		case stepProviderSelection:
			return m.updateProviderSelectionScreen(msg)
		case stepConfigureProvider:
			return m.updateConfigureProviderScreen(msg)
		case stepOllamaURL:
			return m.updateOllamaURLScreen(msg)
		case stepModelSelection:
			return m.updateModelSelectionScreen(msg)
		case stepDataDirectory:
			return m.updateDataDirectoryScreen(msg)
		}

	case urlValidatedMsg:
		m.loading = false
		if msg.valid {
			m.ollamaHost = m.urlInput.Value()
			m.err = ""
			m.providersPingStatus["ollama"] = true

			// Update Ollama provider's BaseURL
			for i := range m.providers {
				if m.providers[i].ID == "ollama" {
					m.providers[i].BaseURL = m.ollamaHost
					break
				}
			}

			// Move to next provider or model selection
			for i := m.currentProviderIdx + 1; i < len(m.providers); i++ {
				if !m.providerCheckboxes[i] {
					continue
				}

				m.currentProviderIdx = i
				m.providers[i].Enabled = true

				switch m.providers[i].ID {
				case "ollama":
					m.step = stepOllamaURL
					m.urlInput.SetValue(m.ollamaHost)
					m.urlInput.Focus()
					return m, textinput.Blink
				default:
					m.step = stepConfigureProvider
					m.apiKeyInput.SetValue("")
					m.apiKeyInput.Focus()
					return m, textinput.Blink
				}
			}

			// No more providers - fetch ALL models
			m.step = stepModelSelection
			m.loading = true
			return m, m.fetchAllProviderModels()
		}

		m.err = fmt.Sprintf("Failed to connect: %v", msg.err)
		return m, nil

	case modelsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = fmt.Sprintf("Failed to load models: %v", msg.err)
			return m, nil
		}
		m.models = msg.models
		if len(m.models) > 0 {
			for i, model := range m.models {
				if strings.Contains(model, m.defaultModel) {
					m.selectedModel = i
					break
				}
			}
		}
		m.err = ""
		return m, nil

	case provider.PingProviderMsg:
		m.loading = false

		if !msg.Valid {
			// Ping failed - show error, stay on API key screen
			m.err = fmt.Sprintf("❌ %v", msg.Err)
			return m, nil
		}

		// Ping succeeded - mark as validated
		m.providersPingStatus[msg.ProviderID] = true
		m.err = ""

		// Move to next enabled provider
		for i := m.currentProviderIdx + 1; i < len(m.providers); i++ {
			if !m.providerCheckboxes[i] {
				continue
			}

			m.currentProviderIdx = i
			m.providers[i].Enabled = true

			switch m.providers[i].ID {
			case "ollama":
				m.step = stepOllamaURL
				m.urlInput.SetValue(m.ollamaHost)
				m.urlInput.Focus()
				return m, textinput.Blink
			default:
				m.step = stepConfigureProvider
				m.apiKeyInput.SetValue("")
				m.apiKeyInput.Focus()
				return m, textinput.Blink
			}
		}

		// No more providers - fetch ALL models
		m.step = stepModelSelection
		m.loading = true
		return m, m.fetchAllProviderModels()

	case provider.SingleProviderModelsMsg:
		// Store models for this provider
		if msg.Err != nil {
			m.err = fmt.Sprintf("Failed to fetch models from %s: %v", msg.ProviderID, msg.Err)
			m.loading = false
			return m, nil
		}

		m.providersModels[msg.ProviderID] = msg.Models

		// Check if all providers returned models
		expectedProviders := 0
		for i := range m.providers {
			if m.providerCheckboxes[i] {
				expectedProviders++
			}
		}

		if len(m.providersModels) >= expectedProviders {
			// Aggregate all models
			var allModels []ollama.ModelInfo
			for _, models := range m.providersModels {
				allModels = append(allModels, models...)
			}

			// Sort alphabetically
			sort.Slice(allModels, func(i, j int) bool {
				return allModels[i].Name < allModels[j].Name
			})

			m.allModels = allModels
			m.loading = false

			// Pre-select llama3.2 if exists
			for i, model := range m.allModels {
				if strings.Contains(strings.ToLower(model.Name), "llama3.2") ||
					strings.Contains(strings.ToLower(model.Name), "llama-3.2") {
					m.selectedModel = i
					break
				}
			}
		}

		return m, nil
	}

	return m, nil
}

func (m WelcomeModel) updateWelcomeScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "alt+q":
		return m, tea.Quit

	case "up", "k":
		if m.selectedButton > 0 {
			m.selectedButton--
		}
		return m, nil

	case "down", "j":
		if m.selectedButton < 2 {
			m.selectedButton++
		}
		return m, nil

	case "enter":
		switch m.selectedButton {
		case 0:
			// Use Defaults
			systemCfg := config.DefaultSystemConfig()
			if err := config.SaveSystemConfig(systemCfg); err != nil {
				m.err = fmt.Sprintf("Failed to save system config: %v", err)
				return m, nil
			}

			dataDir := config.ExpandPath(systemCfg.DataDirectory)
			userCfg := config.DefaultUserConfig()
			if err := config.SaveUserConfig(userCfg, dataDir); err != nil {
				m.err = fmt.Sprintf("Failed to save user config: %v", err)
				return m, nil
			}

			m.step = stepComplete
			return m, tea.Quit

		case 1:
			// Custom Settings
			m.step = stepSecurityMethod
			m.selectedButton = 0
			return m, nil

		case 2:
			// Existing Profile - activate directory picker
			m.existingProfilePicker.Activate()
			return m, m.existingProfilePicker.Picker.Init()
		}
	}

	return m, nil
}

func (m WelcomeModel) updateExistingProfilePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "esc":
		m.existingProfilePicker.Reset()
		return m, nil
	}

	// Update picker with the KeyMsg FIRST
	m.existingProfilePicker.Picker, cmd = m.existingProfilePicker.Picker.Update(msg)

	// Check if a directory was selected
	if m.existingProfilePicker.Picker.Path != "" {
		// Verify it's a directory
		if info, err := os.Stat(m.existingProfilePicker.Picker.Path); err == nil && info.IsDir() {
			dirPath := m.existingProfilePicker.Picker.Path

			// Check if config.toml exists in this directory
			userConfigPath := filepath.Join(dirPath, "config.toml")
			if !config.FileExists(userConfigPath) {
				// Invalid profile directory - show error and stay in picker
				m.err = fmt.Sprintf("No config.toml found in: %s", dirPath)
				m.existingProfilePicker.Picker.Path = ""
				return m, cmd
			}

			// Valid profile directory found
			// First, save system config pointing to this data directory
			systemCfg := &config.SystemConfig{
				DataDirectory: dirPath,
			}
			if err := config.SaveSystemConfig(systemCfg); err != nil {
				m.err = fmt.Sprintf("Failed to save system config: %v", err)
				m.existingProfilePicker.Picker.Path = ""
				return m, cmd
			}

			// Set data directory and complete wizard
			m.dataDirectory = dirPath
			m.existingProfilePicker.Reset()
			m.step = stepComplete
			return m, tea.Quit
		}

		// If it's a file (not directory), clear Path so we don't trigger again
		m.existingProfilePicker.Picker.Path = ""
	}

	return m, cmd
}

func (m WelcomeModel) updateOllamaURLScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "alt+q":
		return m, tea.Quit

	case "esc":
		m.step = stepWelcome
		m.err = ""
		return m, nil

	case "enter":
		if m.urlInput.Value() == "" {
			m.err = "URL cannot be empty"
			return m, nil
		}
		m.loading = true
		m.err = ""
		return m, validateOllamaURL(m.urlInput.Value())

	case "alt+u":
		m.urlInput.SetValue("")
		return m, nil
	}

	m.urlInput, cmd = m.urlInput.Update(msg)
	return m, cmd
}

func (m WelcomeModel) updateModelSelectionScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle filter mode
	if m.modelFilterMode {
		switch msg.String() {
		case "esc":
			m.modelFilterMode = false
			m.modelFilterInput.Blur()
			m.modelFilterInput.SetValue("")
			m.filteredWizardModels = []ollama.ModelInfo{}
			m.selectedModel = 0
			return m, nil

		case "enter":
			displayList := m.allModels
			if m.modelFilterMode && len(m.filteredWizardModels) > 0 {
				displayList = m.filteredWizardModels
			}
			if m.selectedModel >= 0 && m.selectedModel < len(displayList) {
				selectedModelInfo := displayList[m.selectedModel]

				// Store InternalName for API calls and track provider
				m.defaultModel = selectedModelInfo.InternalName
				m.defaultProvider = selectedModelInfo.Provider // Track provider from ModelInfo

				// Go to data directory
				m.step = stepDataDirectory
				m.dirInput.SetValue(m.dataDirectory)
				m.dirInput.Focus()
				m.modelFilterMode = false
				return m, textinput.Blink
			}
			return m, nil

		case "alt+j", "alt+down", "down":
			displayList := m.allModels
			if m.modelFilterMode && len(m.filteredWizardModels) > 0 {
				displayList = m.filteredWizardModels
			}
			if m.selectedModel < len(displayList)-1 {
				m.selectedModel++
			}
			return m, nil

		case "alt+k", "alt+up", "up":
			if m.selectedModel > 0 {
				m.selectedModel--
			}
			return m, nil
		}

		// Update filter input and apply fuzzy search
		var cmd tea.Cmd
		m.modelFilterInput, cmd = m.modelFilterInput.Update(msg)

		filterValue := m.modelFilterInput.Value()
		if filterValue == "" {
			m.filteredWizardModels = m.allModels
		} else {
			targets := make([]string, len(m.allModels))
			for i, mdl := range m.allModels {
				targets[i] = mdl.Name
			}

			matches := fuzzy.Find(filterValue, targets)
			m.filteredWizardModels = make([]ollama.ModelInfo, len(matches))
			for i, match := range matches {
				m.filteredWizardModels[i] = m.allModels[match.Index]
			}
		}

		// Reset selection when filter changes
		if m.selectedModel >= len(m.filteredWizardModels) {
			m.selectedModel = 0
		}

		return m, cmd
	}

	// Normal mode (no filter)
	switch msg.String() {
	case "alt+q":
		return m, tea.Quit

	case "esc":
		m.step = stepOllamaURL
		m.err = ""
		return m, nil

	case "/":
		if !m.modelFilterMode {
			m.modelFilterMode = true
			m.modelFilterInput.Focus()
			m.modelFilterInput.SetValue("")
			m.filteredWizardModels = m.allModels
			m.selectedModel = 0
			return m, nil
		}

	case "up", "k":
		if m.selectedModel > 0 {
			m.selectedModel--
		}
		return m, nil

	case "down", "j":
		displayList := m.allModels
		if m.modelFilterMode && len(m.filteredWizardModels) > 0 {
			displayList = m.filteredWizardModels
		}
		if m.selectedModel < len(displayList)-1 {
			m.selectedModel++
		}
		return m, nil

	case "enter":
		displayList := m.allModels
		if m.modelFilterMode && len(m.filteredWizardModels) > 0 {
			displayList = m.filteredWizardModels
		}
		if len(displayList) > 0 {
			selectedModelInfo := displayList[m.selectedModel]

			// Store InternalName for API calls and track provider
			m.defaultModel = selectedModelInfo.InternalName
			m.defaultProvider = selectedModelInfo.Provider // Track provider from ModelInfo

			// Go to data directory
			m.step = stepDataDirectory
			m.dirInput.SetValue(m.dataDirectory)
			m.dirInput.Focus()
			return m, textinput.Blink
		}
		return m, nil
	}

	return m, nil
}

func (m WelcomeModel) updateDataDirectoryScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "alt+q":
		return m, tea.Quit

	case "esc":
		// Go back to provider selection (or last configured provider)
		m.step = stepProviderSelection
		m.err = ""
		return m, nil

	case "enter":
		if m.dirInput.Value() == "" {
			m.err = "Data directory cannot be empty"
			return m, nil
		}
		m.dataDirectory = m.dirInput.Value()

		systemCfg := &config.SystemConfig{
			DataDirectory: m.dataDirectory,
		}
		if err := config.SaveSystemConfig(systemCfg); err != nil {
			m.err = fmt.Sprintf("Failed to save system config: %v", err)
			return m, nil
		}

		dataDir := config.ExpandPath(m.dataDirectory)

		// Build enabled providers list
		var enabledProviders []config.ProviderConfig
		for i, provider := range m.providers {
			if m.providerCheckboxes[i] {
				provider.Enabled = true
				enabledProviders = append(enabledProviders, provider)
			}
		}

		// Build security config
		securityCfg := config.SecurityConfig{
			CredentialStorage: string(m.securityMethod),
			SSHKeyPath:        m.sshKeyPath,
		}

		userCfg := &config.UserConfig{
			DefaultProvider:  m.defaultProvider, // Top-level provider field
			DefaultModel:     m.defaultModel,    // Top-level model field
			LastUsedProvider: m.defaultProvider, // Initialize to default
			Ollama: config.OllamaConfig{
				Host: m.ollamaHost,
				// DefaultModel removed (migrated to top-level)
			},
			PluginsEnabled: false,
			Security:       securityCfg,
			Providers:      enabledProviders,
		}

		if err := config.SaveUserConfig(userCfg, dataDir); err != nil {
			m.err = fmt.Sprintf("Failed to save user config: %v", err)
			return m, nil
		}

		// Save credentials using CredentialStore
		credStore := config.NewCredentialStore(m.securityMethod, m.sshKeyPath)

		// If using SSH encryption with passphrase, provide it to the credential store
		if m.securityMethod == config.SecuritySSHKey && m.usePassphrase {
			credStore.SetPassphrase(m.passphraseInput.Value())
		}

		// Store all collected API keys
		for providerID, apiKey := range m.providerApiKeys {
			if err := credStore.Set(providerID, apiKey); err != nil {
				m.err = fmt.Sprintf("Failed to store credential for %s: %v", providerID, err)
				return m, nil
			}
		}

		// Save to disk (plaintext or encrypted based on security method)
		if err := credStore.Save(dataDir); err != nil {
			m.err = fmt.Sprintf("Failed to save credentials: %v", err)
			return m, nil
		}

		m.step = stepComplete
		return m, tea.Quit

	case "alt+u":
		m.dirInput.SetValue("")
		return m, nil
	}

	m.dirInput, cmd = m.dirInput.Update(msg)
	return m, cmd
}

func (m WelcomeModel) View() string {
	// Show existing profile picker if active (highest priority)
	if m.existingProfilePicker.Active {
		return RenderFilePickerModal(m.existingProfilePicker, m.width, m.height)
	}

	switch m.step {
	case stepWelcome:
		return m.viewWelcomeScreen()
	case stepSecurityMethod:
		return m.viewSecurityMethodScreen()
	case stepSSHKeySource:
		return m.viewSSHKeySourceScreen()
	case stepCreateOTUIKey:
		return m.viewCreateOTUIKeyScreen()
	case stepSetPassphrase:
		return m.viewSetPassphraseScreen()
	case stepSelectExistingKey:
		return m.viewSelectExistingKeyScreen()
	case stepVerifyExistingKeyPassphrase:
		return m.viewVerifyExistingKeyPassphraseScreen()
	case stepBackupReminder:
		return m.viewBackupReminderScreen()
	case stepProviderSelection:
		return m.viewProviderSelectionScreen()
	case stepConfigureProvider:
		return m.viewConfigureProviderScreen()
	case stepOllamaURL:
		return m.viewOllamaURLScreen()
	case stepModelSelection:
		return m.viewModelSelectionScreen()
	case stepDataDirectory:
		return m.viewDataDirectoryScreen()
	}
	return ""
}

func (m WelcomeModel) viewWelcomeScreen() string {
	var sb strings.Builder

	for _, line := range strings.Split(ASCIIArt, "\n") {
		sb.WriteString(titleStyle.Render(line))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	sb.WriteString("\n\n")
	if m.isRestartScenario {
		sb.WriteString("Let's get you setup!")
		sb.WriteString("\n\n\n")
		goto continueRendering
	}

	sb.WriteString("It looks like you are running otui for the\n")
	sb.WriteString("first time. Let's get you setup!")
	sb.WriteString("\n\n\n")

continueRendering:

	var button1, button2, button3 string

	switch m.selectedButton {
	case 0:
		button1 = selectedButtonStyle.Render("Use Defaults")
		button2 = buttonStyle.Render("Custom Settings")
		button3 = buttonStyle.Render("Existing Profile")
	case 1:
		button1 = buttonStyle.Render("Use Defaults")
		button2 = selectedButtonStyle.Render("Custom Settings")
		button3 = buttonStyle.Render("Existing Profile")
	case 2:
		button1 = buttonStyle.Render("Use Defaults")
		button2 = buttonStyle.Render("Custom Settings")
		button3 = selectedButtonStyle.Render("Existing Profile")
	}

	buttons := lipgloss.JoinVertical(lipgloss.Left, button1, button2, button3)
	sb.WriteString(buttons)
	sb.WriteString("\n\n")

	hint := "Press ↑/↓ or j/k to switch • Enter to select • Alt+Q Quit"
	sb.WriteString(featureStyle.Render(hint))
	sb.WriteString("\n")

	if m.err != "" {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render(m.err))
	}

	content := sb.String()
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m WelcomeModel) viewOllamaURLScreen() string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString(centerText(titleStyle.Render("Ollama Server Configuration"), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("Enter the URL of your Ollama server:"), m.width))
	sb.WriteString("\n\n")

	sb.WriteString(centerText(inputStyle.Render(m.urlInput.View()), m.width))
	sb.WriteString("\n\n\n")

	if m.loading {
		sb.WriteString(centerText(featureStyle.Render("⏳ Validating connection..."), m.width))
	} else {
		sb.WriteString(centerText(featureStyle.Render("Alt+U Clear • Enter Continue • Esc Back • Alt+Q Quit"), m.width))
	}

	if m.err != "" {
		sb.WriteString("\n\n")
		sb.WriteString(centerText(errorStyle.Render(m.err), m.width))
	}

	return sb.String()
}

func (m WelcomeModel) viewModelSelectionScreen() string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString(centerText(titleStyle.Render("Select Default Model (All Providers)"), m.width))
	sb.WriteString("\n\n\n")

	if m.loading {
		sb.WriteString(centerText(featureStyle.Render("⏳ Loading models..."), m.width))
		return sb.String()
	}

	if len(m.allModels) == 0 {
		sb.WriteString(centerText(errorStyle.Render("No models found!"), m.width))
		sb.WriteString("\n\n")
		sb.WriteString(centerText(featureStyle.Render("Press Esc to go back • Alt+Q Quit"), m.width))
		return sb.String()
	}

	// Show filter input or model count
	var header string
	displayList := m.allModels
	if m.modelFilterMode && len(m.filteredWizardModels) > 0 {
		displayList = m.filteredWizardModels
	}

	if m.modelFilterMode {
		header = m.modelFilterInput.View()
	} else {
		if len(m.allModels) == len(displayList) {
			header = fmt.Sprintf("%d models", len(m.allModels))
		} else {
			header = fmt.Sprintf("%d of %d models", len(displayList), len(m.allModels))
		}
	}
	sb.WriteString(centerText(header, m.width))
	sb.WriteString("\n\n")

	// Determine if we have multiple providers
	multiProvider := len(m.providersModels) > 1

	// Calculate viewport for scrolling (following appview_selector.go pattern)
	maxLines := m.height - 12 // Reserve space for title, header, footer, hints
	startIdx := 0
	endIdx := len(displayList)

	// Apply scrolling if model list exceeds viewport
	if len(displayList) > maxLines {
		if m.selectedModel < maxLines/2 {
			// Near top - show from beginning
			endIdx = maxLines
		} else if m.selectedModel >= len(displayList)-maxLines/2 {
			// Near bottom - show last maxLines
			startIdx = len(displayList) - maxLines
		} else {
			// Middle - keep selected centered
			startIdx = m.selectedModel - maxLines/2
			endIdx = startIdx + maxLines
		}
	}

	// Render only models in viewport
	for i := startIdx; i < endIdx && i < len(displayList); i++ {
		model := displayList[i]

		// Format with provider suffix if multiple providers
		displayName := model.Name
		if multiProvider && model.Provider != "" {
			displayName = fmt.Sprintf("%s (%s)", model.Name, model.Provider)
		}

		var line string
		if i == m.selectedModel {
			line = selectedButtonStyle.Render(fmt.Sprintf("→ %s", displayName))
		} else {
			line = featureStyle.Render(fmt.Sprintf("  %s", displayName))
		}
		sb.WriteString(centerText(line, m.width))
		sb.WriteString("\n")
	}

	sb.WriteString("\n\n")

	// Footer hints
	var footerText string
	if m.modelFilterMode {
		footerText = FormatFooter(
			"Type", "to filter",
			"Alt+J/K", "Navigate",
			"Enter", "Select",
			"Esc", "Cancel",
		)
	} else {
		footerText = FormatFooter(
			"/", "Filter",
			"j/k", "Navigate",
			"Enter", "Select",
			"Esc", "Back",
			"Alt+Q", "Quit",
		)
	}
	sb.WriteString(centerText(featureStyle.Render(footerText), m.width))

	if m.err != "" {
		sb.WriteString("\n\n")
		sb.WriteString(centerText(errorStyle.Render(m.err), m.width))
	}

	return sb.String()
}

func (m WelcomeModel) viewDataDirectoryScreen() string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString(centerText(titleStyle.Render("Data Directory"), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("Where should otui store configs and sessions data?"), m.width))
	sb.WriteString("\n\n")

	sb.WriteString(centerText(inputStyle.Render(m.dirInput.View()), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("Alt+U Clear • Enter Finish • Esc Back • Alt+Q Quit"), m.width))

	if m.err != "" {
		sb.WriteString("\n\n")
		sb.WriteString(centerText(errorStyle.Render(m.err), m.width))
	}

	return sb.String()
}

func centerText(text string, width int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > 1 {
		var sb strings.Builder
		for i, line := range lines {
			sb.WriteString(centerText(line, width))
			if i < len(lines)-1 {
				sb.WriteString("\n")
			}
		}
		return sb.String()
	}

	textWidth := lipgloss.Width(text)
	if textWidth >= width {
		return text
	}

	padding := (width - textWidth) / 2
	return strings.Repeat(" ", padding) + text
}

// ===== Security Method Screen =====

func (m WelcomeModel) updateSecurityMethodScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "alt+q":
		return m, tea.Quit
	case "esc":
		m.step = stepWelcome
		m.err = ""
		return m, nil
	case "up", "k":
		m.selectedButton = 0
		return m, nil
	case "down", "j":
		m.selectedButton = 1
		return m, nil
	case "enter":
		switch m.selectedButton {
		case 0:
			m.securityMethod = config.SecurityPlainText
			m.step = stepProviderSelection
		case 1:
			m.securityMethod = config.SecuritySSHKey
			m.step = stepSSHKeySource
		}
		m.selectedButton = 0
		return m, nil
	}
	return m, nil
}

func (m WelcomeModel) viewSecurityMethodScreen() string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString(centerText(titleStyle.Render("Security Method"), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("How should OTUI protect your data?"), m.width))
	sb.WriteString("\n\n")

	var opt1, opt2 string
	if m.selectedButton == 0 {
		opt1 = selectedButtonStyle.Render("→ Plain Text (local only)")
		opt2 = featureStyle.Render("  SSH Key (recommended for cloud)")
	} else {
		opt1 = featureStyle.Render("  Plain Text (local only)")
		opt2 = selectedButtonStyle.Render("→ SSH Key (recommended for cloud)")
	}

	sb.WriteString(centerText(opt1, m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("    Simple, fast, no encryption"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("    Use for: Local Ollama setups"), m.width))
	sb.WriteString("\n\n")

	sb.WriteString(centerText(opt2, m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("    Secure, encrypted with SSH key"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("    Use for: Cloud providers, sensitive data"), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("j/k Navigate • Enter Select • Esc Back • Alt+Q Quit"), m.width))

	if m.err != "" {
		sb.WriteString("\n\n")
		sb.WriteString(centerText(errorStyle.Render(m.err), m.width))
	}

	return sb.String()
}

// ===== SSH Key Source Screen =====

func (m WelcomeModel) updateSSHKeySourceScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "alt+q":
		return m, tea.Quit
	case "esc":
		m.step = stepSecurityMethod
		m.err = ""
		return m, nil
	case "up", "k":
		m.sshKeySource = 0
		return m, nil
	case "down", "j":
		m.sshKeySource = 1
		return m, nil
	case "enter":
		if m.sshKeySource == 0 {
			m.step = stepCreateOTUIKey
			m.selectedButton = 0
		} else {
			// Load available SSH keys
			keys, err := config.FindSSHKeys()
			if err != nil {
				m.err = fmt.Sprintf("Error finding SSH keys: %v", err)
				return m, nil
			}
			if len(keys) == 0 {
				m.err = "No SSH keys found. Please create one first."
				return m, nil
			}
			m.availableKeys = keys
			m.selectedKeyIndex = 0
			m.step = stepSelectExistingKey
		}
		return m, nil
	}
	return m, nil
}

func (m WelcomeModel) viewSSHKeySourceScreen() string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString(centerText(titleStyle.Render("SSH Key Method"), m.width))
	sb.WriteString("\n\n\n")

	var opt1, opt2 string
	if m.sshKeySource == 0 {
		opt1 = selectedButtonStyle.Render("→ Create new OTUI key (recommended)")
		opt2 = featureStyle.Render("  Use existing SSH key")
	} else {
		opt1 = featureStyle.Render("  Create new OTUI key (recommended)")
		opt2 = selectedButtonStyle.Render("→ Use existing SSH key")
	}

	sb.WriteString(centerText(opt1, m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("    Dedicated key just for OTUI"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("    Optional passphrase protection"), m.width))
	sb.WriteString("\n\n")

	sb.WriteString(centerText(opt2, m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("    Reuse your current SSH key"), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("j/k Navigate • Enter Select • Esc Back • Alt+Q Quit"), m.width))

	if m.err != "" {
		sb.WriteString("\n\n")
		sb.WriteString(centerText(errorStyle.Render(m.err), m.width))
	}

	return sb.String()
}

// ===== Create OTUI Key Screen =====

func (m WelcomeModel) updateCreateOTUIKeyScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "alt+q":
		return m, tea.Quit
	case "esc":
		m.step = stepSSHKeySource
		m.err = ""
		return m, nil
	case "up", "k":
		m.selectedButton = 0
		return m, nil
	case "down", "j":
		m.selectedButton = 1
		return m, nil
	case "enter":
		m.usePassphrase = (m.selectedButton == 1)

		if m.usePassphrase {
			m.passphraseInput.Focus()
			m.step = stepSetPassphrase
			return m, textinput.Blink
		}

		// Create key without passphrase
		keyPath, err := config.CreateOTUIKey("")
		if err != nil {
			m.err = fmt.Sprintf("Failed to create SSH key: %v", err)
			return m, nil
		}

		m.sshKeyPath = keyPath
		m.step = stepBackupReminder
		return m, nil
	}
	return m, nil
}

func (m WelcomeModel) viewCreateOTUIKeyScreen() string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString(centerText(titleStyle.Render("Create OTUI SSH Key"), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("OTUI will create a new SSH key pair"), m.width))
	sb.WriteString("\n\n")
	sb.WriteString(centerText(featureStyle.Render("Type: ED25519 (modern, secure)"), m.width))
	sb.WriteString("\n\n")

	sb.WriteString(centerText(featureStyle.Render("Protect key with passphrase? (optional)"), m.width))
	sb.WriteString("\n\n")

	var opt1, opt2 string
	if m.selectedButton == 0 {
		opt1 = selectedButtonStyle.Render("→ No passphrase (zero friction)")
		opt2 = featureStyle.Render("  Yes, set passphrase (more secure)")
	} else {
		opt1 = featureStyle.Render("  No passphrase (zero friction)")
		opt2 = selectedButtonStyle.Render("→ Yes, set passphrase (more secure)")
	}

	sb.WriteString(centerText(opt1, m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(opt2, m.width))
	sb.WriteString("\n\n")

	sb.WriteString(centerText(featureStyle.Render("ℹ️  This key is ONLY for encrypting your"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("   OTUI data (credentials, sessions)."), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("j/k Navigate • Enter Select • Esc Back • Alt+Q Quit"), m.width))

	if m.err != "" {
		sb.WriteString("\n\n")
		sb.WriteString(centerText(errorStyle.Render(m.err), m.width))
	}

	return sb.String()
}

// ===== Set Passphrase Screen =====

func (m WelcomeModel) updateSetPassphraseScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "alt+q":
		return m, tea.Quit
	case "esc":
		m.step = stepCreateOTUIKey
		m.passphraseInput.SetValue("")
		m.passphraseConfirm.SetValue("")
		m.err = ""
		return m, nil
	case "tab":
		m.activePassphraseField = 1 - m.activePassphraseField
		if m.activePassphraseField == 0 {
			m.passphraseInput.Focus()
			m.passphraseConfirm.Blur()
		} else {
			m.passphraseInput.Blur()
			m.passphraseConfirm.Focus()
		}
		return m, textinput.Blink
	case "enter":
		pass := m.passphraseInput.Value()
		confirm := m.passphraseConfirm.Value()

		if err := ValidatePassphraseNotEmpty(pass); err != nil {
			m.err = GetEmptyPassphraseError()
			return m, nil
		}

		if pass != confirm {
			m.err = "Passphrases do not match"
			return m, nil
		}

		// Create key with passphrase
		keyPath, err := config.CreateOTUIKey(pass)
		if err != nil {
			m.err = fmt.Sprintf("Failed to create SSH key: %v", err)
			return m, nil
		}

		m.sshKeyPath = keyPath
		m.step = stepBackupReminder
		return m, nil
	}

	if m.activePassphraseField == 0 {
		m.passphraseInput, cmd = m.passphraseInput.Update(msg)
	} else {
		m.passphraseConfirm, cmd = m.passphraseConfirm.Update(msg)
	}
	return m, cmd
}

func (m WelcomeModel) viewSetPassphraseScreen() string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString(centerText(titleStyle.Render("Set Key Passphrase"), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("Enter passphrase:"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(inputStyle.Render(m.passphraseInput.View()), m.width))
	sb.WriteString("\n\n")

	sb.WriteString(centerText(featureStyle.Render("Confirm passphrase:"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(inputStyle.Render(m.passphraseConfirm.View()), m.width))
	sb.WriteString("\n\n")

	sb.WriteString(centerText(featureStyle.Render("This passphrase will be required when:"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("• Starting OTUI"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("• Accessing encrypted data"), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("Tab Next Field • Enter Continue • Esc Cancel • Alt+Q Quit"), m.width))

	if m.err != "" {
		sb.WriteString("\n\n")
		sb.WriteString(centerText(errorStyle.Render(m.err), m.width))
	}

	return sb.String()
}

// ===== Select Existing Key Screen =====

func (m WelcomeModel) updateSelectExistingKeyScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "alt+q":
		return m, tea.Quit
	case "esc":
		m.step = stepSSHKeySource
		m.err = ""
		return m, nil
	case "up", "k":
		if m.selectedKeyIndex > 0 {
			m.selectedKeyIndex--
		}
		return m, nil
	case "down", "j":
		if m.selectedKeyIndex < len(m.availableKeys)-1 {
			m.selectedKeyIndex++
		}
		return m, nil
	case "enter":
		if len(m.availableKeys) == 0 {
			m.err = "No SSH keys found"
			return m, nil
		}
		m.sshKeyPath = m.availableKeys[m.selectedKeyIndex]

		// Check if key is encrypted
		_, err := config.LoadSSHPrivateKey(m.sshKeyPath)
		if err != nil && strings.Contains(err.Error(), "passphrase required") {
			// Key is encrypted - prompt for passphrase
			m.step = stepVerifyExistingKeyPassphrase
			m.existingKeyPassphrase.Focus()
			m.err = ""
			return m, textinput.Blink
		} else if err != nil {
			// Other error (invalid key, file read error, etc.)
			m.err = fmt.Sprintf("Invalid SSH key: %v", err)
			return m, nil
		}

		// Key is not encrypted - continue to backup reminder
		m.step = stepBackupReminder
		m.selectedButton = 0
		return m, nil
	}
	return m, nil
}

func (m WelcomeModel) viewSelectExistingKeyScreen() string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString(centerText(titleStyle.Render("Select Existing SSH Key"), m.width))
	sb.WriteString("\n\n\n")

	for i, key := range m.availableKeys {
		var line string
		if i == m.selectedKeyIndex {
			line = selectedButtonStyle.Render(fmt.Sprintf("→ %s", key))
		} else {
			line = featureStyle.Render(fmt.Sprintf("  %s", key))
		}
		sb.WriteString(centerText(line, m.width))
		sb.WriteString("\n")
	}

	sb.WriteString("\n\n")
	sb.WriteString(centerText(featureStyle.Render("ℹ️  Note: If key is passphrase-protected,"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("   you'll be prompted each time you start OTUI."), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("j/k Navigate • Enter Select • Esc Back • Alt+Q Quit"), m.width))

	if m.err != "" {
		sb.WriteString("\n\n")
		sb.WriteString(centerText(errorStyle.Render(m.err), m.width))
	}

	return sb.String()
}

// ===== Backup Reminder Screen =====

func (m WelcomeModel) updateBackupReminderScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "alt+q":
		return m, tea.Quit
	case "g":
		m.step = stepProviderSelection
		m.selectedButton = 0
		return m, nil
	}
	return m, nil
}

func (m WelcomeModel) updateVerifyExistingKeyPassphraseScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "alt+q":
		return m, tea.Quit

	case "esc":
		// Go back to key selection
		m.step = stepSelectExistingKey
		m.existingKeyPassphrase.SetValue("")
		m.err = ""
		return m, nil

	case "enter":
		passphrase := m.existingKeyPassphrase.Value()
		if err := ValidatePassphraseNotEmpty(passphrase); err != nil {
			m.err = "Passphrase cannot be empty (or press Esc to select a different key)"
			return m, nil
		}

		// Validate passphrase by attempting to load the key
		_, err := config.LoadSSHPrivateKeyWithPassphrase(m.sshKeyPath, passphrase)
		if err != nil {
			m.err = GetIncorrectPassphraseError()
			m.existingKeyPassphrase.SetValue("")
			return m, textinput.Blink
		}

		// Passphrase is correct - continue to backup reminder
		m.step = stepBackupReminder
		m.existingKeyPassphrase.SetValue("") // Clear for security
		m.err = ""
		return m, nil

	case "alt+u":
		m.existingKeyPassphrase.SetValue("")
		return m, nil
	}

	m.existingKeyPassphrase, cmd = m.existingKeyPassphrase.Update(msg)
	return m, cmd
}

func (m WelcomeModel) viewVerifyExistingKeyPassphraseScreen() string {
	return RenderPassphraseModal(
		"SSH Key Passphrase",
		m.sshKeyPath,
		m.existingKeyPassphrase,
		m.err,
		m.width,
		m.height,
	)
}

func (m WelcomeModel) viewBackupReminderScreen() string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString(centerText(titleStyle.Render("⚠️  IMPORTANT: Back Up Your Key!"), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("Your OTUI encryption key:"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(selectedButtonStyle.Render(m.sshKeyPath), m.width))
	sb.WriteString("\n\n")

	sb.WriteString(centerText(errorStyle.Render("Without this key, you cannot:"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("• Decrypt your API credentials"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("• Access encrypted sessions"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("• Use OTUI on other devices"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("• Recover if this machine fails"), m.width))
	sb.WriteString("\n\n")

	sb.WriteString(centerText(featureStyle.Render("Backup methods:"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("• Copy to USB drive"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("• Store in password manager"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("• Save to encrypted cloud storage"), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("g Got it! • Alt+Q Quit"), m.width))

	return sb.String()
}

// ===== Provider Selection Screen =====

func (m WelcomeModel) updateProviderSelectionScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "alt+q":
		return m, tea.Quit
	case "esc":
		if m.securityMethod == config.SecuritySSHKey {
			m.step = stepSSHKeySource
		} else {
			m.step = stepSecurityMethod
		}
		m.err = ""
		return m, nil
	case "up", "k":
		if m.selectedButton > 0 {
			m.selectedButton--
		}
		return m, nil
	case "down", "j":
		if m.selectedButton < len(m.providers) {
			m.selectedButton++
		}
		return m, nil
	case " ":
		// Toggle checkbox (space key)
		if m.selectedButton < len(m.providers) {
			m.providerCheckboxes[m.selectedButton] = !m.providerCheckboxes[m.selectedButton]
		}
		return m, nil
	case "enter":
		// Check if any providers are selected
		anySelected := false
		for _, checked := range m.providerCheckboxes {
			if checked {
				anySelected = true
				break
			}
		}

		if !anySelected {
			m.err = "Please select at least one provider"
			return m, nil
		}

		// Move to configure first enabled provider
		for i, checked := range m.providerCheckboxes {
			if checked {
				m.currentProviderIdx = i
				m.providers[i].Enabled = true

				// If Ollama, go to Ollama URL screen
				if m.providers[i].ID == "ollama" {
					m.step = stepOllamaURL
					m.urlInput.SetValue(m.ollamaHost)
					m.urlInput.Focus()
					return m, textinput.Blink
				}

				// Otherwise configure provider (API key)
				m.step = stepConfigureProvider
				m.apiKeyInput.SetValue("")
				m.apiKeyInput.Focus()
				return m, textinput.Blink
			}
		}

		// If we get here, go to data directory
		m.step = stepDataDirectory
		m.dirInput.SetValue(m.dataDirectory)
		m.dirInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m WelcomeModel) viewProviderSelectionScreen() string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString(centerText(titleStyle.Render("Select Providers"), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("Which AI providers would you like to use?"), m.width))
	sb.WriteString("\n")
	sb.WriteString(centerText(featureStyle.Render("(Select at least one)"), m.width))
	sb.WriteString("\n\n")

	for i, provider := range m.providers {
		checkbox := "[ ]"
		if m.providerCheckboxes[i] {
			checkbox = "[✓]"
		}

		var line string
		if i == m.selectedButton {
			line = selectedButtonStyle.Render(fmt.Sprintf("→ %s %s", checkbox, provider.Name))
		} else {
			line = featureStyle.Render(fmt.Sprintf("  %s %s", checkbox, provider.Name))
		}
		sb.WriteString(centerText(line, m.width))
		sb.WriteString("\n")
	}

	sb.WriteString("\n\n")
	sb.WriteString(centerText(featureStyle.Render("j/k Navigate • Space Toggle • Enter Continue • Esc Back • Alt+Q Quit"), m.width))

	if m.err != "" {
		sb.WriteString("\n\n")
		sb.WriteString(centerText(errorStyle.Render(m.err), m.width))
	}

	return sb.String()
}

// ===== Configure Provider Screen =====

func (m WelcomeModel) updateConfigureProviderScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "alt+q":
		return m, tea.Quit
	case "esc":
		m.step = stepProviderSelection
		m.err = ""
		return m, nil
	case "enter":
		prov := m.providers[m.currentProviderIdx]
		apiKey := m.apiKeyInput.Value()

		if apiKey == "" {
			m.err = "API key cannot be empty"
			return m, nil
		}

		// Store API key
		m.providerApiKeys[prov.ID] = apiKey

		// Ping provider to validate (business logic in provider package!)
		m.loading = true
		m.err = ""
		return m, provider.PingProvider(prov.ID, prov.BaseURL, apiKey)
	}

	m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
	return m, cmd
}

func (m WelcomeModel) viewConfigureProviderScreen() string {
	var sb strings.Builder
	provider := m.providers[m.currentProviderIdx]

	sb.WriteString("\n\n")
	sb.WriteString(centerText(titleStyle.Render(fmt.Sprintf("Configure %s", provider.Name)), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("Enter your API key:"), m.width))
	sb.WriteString("\n\n")

	sb.WriteString(centerText(inputStyle.Render(m.apiKeyInput.View()), m.width))
	sb.WriteString("\n\n\n")

	sb.WriteString(centerText(featureStyle.Render("Enter Continue • Esc Back • Alt+Q Quit"), m.width))

	if m.err != "" {
		sb.WriteString("\n\n")
		sb.WriteString(centerText(errorStyle.Render(m.err), m.width))
	}

	return sb.String()
}

func (m WelcomeModel) IsComplete() bool {
	return m.step == stepComplete
}
