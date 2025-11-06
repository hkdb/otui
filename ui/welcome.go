package ui

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"otui/config"
	"otui/ollama"
)

type wizardStep int

const (
	stepWelcome wizardStep = iota
	stepOllamaURL
	stepModelSelection
	stepDataDirectory
	stepComplete
)

type WelcomeModel struct {
	step           wizardStep
	selectedButton int

	ollamaHost    string
	defaultModel  string
	dataDirectory string

	urlInput      textinput.Model
	dirInput      textinput.Model
	models        []string
	selectedModel int

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

	return WelcomeModel{
		step:           stepWelcome,
		selectedButton: 0,
		ollamaHost:     "http://localhost:11434",
		defaultModel:   "llama3.2:latest",
		dataDirectory:  "~/.local/share/otui",
		urlInput:       urlInput,
		dirInput:       dirInput,
		models:         []string{},
		selectedModel:  0,
	}
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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch m.step {
		case stepWelcome:
			return m.updateWelcomeScreen(msg)
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
			m.step = stepModelSelection
			m.loading = true
			return m, loadModels(m.ollamaHost)
		} else {
			m.err = fmt.Sprintf("Failed to connect: %v", msg.err)
			return m, nil
		}

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
	}

	return m, nil
}

func (m WelcomeModel) updateWelcomeScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "up", "k":
		m.selectedButton = 0
		return m, nil

	case "down", "j":
		m.selectedButton = 1
		return m, nil

	case "enter":
		if m.selectedButton == 0 {
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
		} else {
			m.step = stepOllamaURL
			m.urlInput.SetValue(m.ollamaHost)
			m.urlInput.Focus()
			return m, textinput.Blink
		}
	}

	return m, nil
}

func (m WelcomeModel) updateOllamaURLScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "q":
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
	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc":
		m.step = stepOllamaURL
		m.err = ""
		return m, nil

	case "up", "k":
		if m.selectedModel > 0 {
			m.selectedModel--
		}
		return m, nil

	case "down", "j":
		if m.selectedModel < len(m.models)-1 {
			m.selectedModel++
		}
		return m, nil

	case "enter":
		if len(m.models) > 0 {
			m.defaultModel = m.models[m.selectedModel]
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
	case "q":
		return m, tea.Quit

	case "esc":
		m.step = stepModelSelection
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
		userCfg := &config.UserConfig{
			Ollama: config.OllamaConfig{
				Host:         m.ollamaHost,
				DefaultModel: m.defaultModel,
			},
		}
		if err := config.SaveUserConfig(userCfg, dataDir); err != nil {
			m.err = fmt.Sprintf("Failed to save user config: %v", err)
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
	switch m.step {
	case stepWelcome:
		return m.viewWelcomeScreen()
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

	for _, feature := range Features {
		sb.WriteString(featureStyle.Render(feature))
		sb.WriteString("\n")
	}

	sb.WriteString("\n\n")
	sb.WriteString("It looks like you are running otui for the\n")
	sb.WriteString("first time. Let's get you setup!")
	sb.WriteString("\n\n\n")

	var button1, button2 string

	if m.selectedButton == 0 {
		button1 = selectedButtonStyle.Render("Use Defaults")
		button2 = buttonStyle.Render("Custom Settings")
	} else {
		button1 = buttonStyle.Render("Use Defaults")
		button2 = selectedButtonStyle.Render("Custom Settings")
	}

	buttons := lipgloss.JoinVertical(lipgloss.Left, button1, button2)
	sb.WriteString(buttons)
	sb.WriteString("\n\n")

	hint := "Press ↑/↓ or j/k to switch • Enter to select • q to exit"
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
		sb.WriteString(centerText(featureStyle.Render("Alt+U Clear • Enter Continue • Esc Back • q Exit"), m.width))
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
	sb.WriteString(centerText(titleStyle.Render("Select Default Model"), m.width))
	sb.WriteString("\n\n\n")

	if m.loading {
		sb.WriteString(centerText(featureStyle.Render("⏳ Loading models..."), m.width))
		return sb.String()
	}

	if len(m.models) == 0 {
		sb.WriteString(centerText(errorStyle.Render("No models found!"), m.width))
		sb.WriteString("\n\n")
		sb.WriteString(centerText(featureStyle.Render("Press Esc to go back • q to exit"), m.width))
		return sb.String()
	}

	sb.WriteString(centerText(featureStyle.Render("Use ↑/↓ or j/k to navigate:"), m.width))
	sb.WriteString("\n\n")

	for i, model := range m.models {
		var line string
		if i == m.selectedModel {
			line = selectedButtonStyle.Render(fmt.Sprintf("→ %s", model))
		} else {
			line = featureStyle.Render(fmt.Sprintf("  %s", model))
		}
		sb.WriteString(centerText(line, m.width))
		sb.WriteString("\n")
	}

	sb.WriteString("\n\n")
	sb.WriteString(centerText(featureStyle.Render("Press Enter to continue • Esc to go back • q to exit"), m.width))

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

	sb.WriteString(centerText(featureStyle.Render("Alt+U Clear • Enter Finish • Esc Back • q Exit"), m.width))

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

func (m WelcomeModel) IsComplete() bool {
	return m.step == stepComplete
}
