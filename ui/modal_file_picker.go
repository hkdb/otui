package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	otuiconfig "otui/config"
)

type FilePickerMode int

const (
	FilePickerModeOpen FilePickerMode = iota
	FilePickerModeSave
)

type FilePickerConfig struct {
	Title          string
	Mode           FilePickerMode
	AllowedTypes   []string
	StartDirectory string
	ShowHidden     bool
	OperationType  string // "Import" or "Export" for dynamic processing titles
}

type FilePickerState struct {
	Active     bool
	Picker     filepicker.Model
	Config     FilePickerConfig
	Processing bool
	CleaningUp bool
	Spinner    spinner.Model
	Success    *string
}

func NewFilePickerState(config FilePickerConfig) FilePickerState {
	fp := filepicker.New()
	fp.AllowedTypes = config.AllowedTypes
	fp.Height = 10
	fp.DirAllowed = true
	fp.FileAllowed = true
	fp.ShowPermissions = false
	fp.ShowSize = false
	fp.ShowHidden = config.ShowHidden

	startDir := config.StartDirectory
	if startDir == "" {
		startDir = otuiconfig.GetHomeDir()
	}
	fp.CurrentDirectory = startDir

	fp.Styles.Directory = lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true)
	fp.Styles.File = lipgloss.NewStyle().
		Foreground(lipgloss.Color("15"))
	fp.Styles.Selected = lipgloss.NewStyle().
		Foreground(successColor).
		Bold(true)
	fp.Styles.Cursor = lipgloss.NewStyle().
		Foreground(successColor)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return FilePickerState{
		Active:  false,
		Picker:  fp,
		Config:  config,
		Spinner: sp,
	}
}

func (fps *FilePickerState) Activate() {
	fps.Active = true
	fps.Processing = false
	fps.CleaningUp = false
	fps.Success = nil
}

func (fps *FilePickerState) Reset() {
	fps.Active = false
	fps.Processing = false
	fps.CleaningUp = false
	fps.Success = nil
}

func RenderFilePickerModal(state FilePickerState, width, height int) string {
	if otuiconfig.DebugLog != nil {
		otuiconfig.DebugLog.Printf("RENDER: Success=%v, CleaningUp=%v, Processing=%v, Active=%v",
			state.Success != nil, state.CleaningUp, state.Processing, state.Active)
	}

	if state.Success != nil {
		return renderFilePickerSuccess(*state.Success, state.Config.Title, width, height)
	}

	if state.CleaningUp {
		return renderFilePickerCleanup(state.Spinner, width, height)
	}

	if state.Processing {
		return renderFilePickerProcessing(state.Spinner, state.Config, width, height)
	}

	return renderFilePickerInput(state.Picker, state.Config.Title, width, height)
}

func renderFilePickerInput(picker filepicker.Model, title string, width, height int) string {
	// Guard clause: prevent rendering in tiny terminals
	if width < 20 || height < 10 {
		return "Terminal too small"
	}

	modalWidth := width - 10
	if modalWidth < 10 {
		modalWidth = 10
	}
	if modalWidth > 80 {
		modalWidth = 80
	}

	// Build message lines with file picker content
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Top padding

	pickerView := picker.View()
	pickerLines := strings.Split(pickerView, "\n")

	contentStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Left)

	for _, line := range pickerLines {
		trimmedLine := strings.TrimRight(line, " ")
		styledLine := contentStyle.Render("  " + trimmedLine)
		messageLines = append(messageLines, styledLine)
	}

	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Bottom padding

	// Format footer
	footer := "j/k Navigate  h/l Back/Forward  Esc Cancel"

	return RenderThreeSectionModal(
		title,
		messageLines,
		footer,
		ModalTypeInfo,
		modalWidth,
		width,
		height,
	)
}

func renderFilePickerProcessing(sp spinner.Model, config FilePickerConfig, width, height int) string {
	// Guard clause: prevent rendering in tiny terminals
	if width < 20 || height < 10 {
		return "Terminal too small"
	}

	modalWidth := width - 10
	if modalWidth < 10 {
		modalWidth = 10
	}
	if modalWidth > 80 {
		modalWidth = 80
	}

	// Build dynamic title
	title := "Processing " + config.OperationType

	// Build message lines with spinner
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Top padding

	processingLine := fmt.Sprintf("%s Processing...", sp.View())
	styledProcessing := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Bold(true).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render(processingLine)

	messageLines = append(messageLines, styledProcessing)
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Bottom padding

	// Format footer
	footer := "Press Esc to cancel"

	return RenderThreeSectionModal(
		title,
		messageLines,
		footer,
		ModalTypeInfo,
		modalWidth,
		width,
		height,
	)
}

func renderFilePickerCleanup(sp spinner.Model, width, height int) string {
	// Guard clause: prevent rendering in tiny terminals
	if width < 20 || height < 10 {
		return "Terminal too small"
	}

	modalWidth := width - 10
	if modalWidth < 10 {
		modalWidth = 10
	}
	if modalWidth > 80 {
		modalWidth = 80
	}

	// Build content with spacing
	var contentLines []string
	contentLines = append(contentLines, strings.Repeat(" ", modalWidth)) // Top padding

	cleanupLine := fmt.Sprintf("%s Cleaning up...", sp.View())
	styledCleanup := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Bold(true).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render(cleanupLine)

	contentLines = append(contentLines, styledCleanup)
	contentLines = append(contentLines, strings.Repeat(" ", modalWidth)) // Bottom padding

	// Apply BorderTop and BorderBottom
	content := lipgloss.NewStyle().
		BorderTop(true).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(contentLines, "\n"))

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func renderFilePickerSuccess(path string, title string, width, height int) string {
	// Guard clause: prevent rendering in tiny terminals
	if width < 20 || height < 10 {
		return "Terminal too small"
	}

	modalWidth := 70
	if width < modalWidth+10 {
		modalWidth = width - 10
		if modalWidth < 10 {
			modalWidth = 10
		}
	}

	// Title is dynamic: "✓ Import Successful" or "✓ Export Successful"
	successTitle := "✓ " + title + " Successful"

	// Build message lines with file path
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Top padding

	wrappedMsg := wordWrap(path, modalWidth-4)
	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Foreground(accentColor).
		Align(lipgloss.Left)

	for _, line := range strings.Split(wrappedMsg, "\n") {
		styledLine := messageStyle.Render("  " + line)
		messageLines = append(messageLines, styledLine)
	}

	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Bottom padding

	// Format footer
	footer := "Press Enter to acknowledge"

	// Use RenderThreeSectionModal but we need successColor for title
	// Since ModalTypeInfo uses accentColor, we'll need custom rendering for title
	// Let's check if we can override...actually, let's just use the helper and accept accentColor
	// Or we can inline a modified version. For consistency, let's use custom title rendering.

	// Actually, looking at the helper, we can't easily override title color to successColor
	// Let's do custom rendering but follow the 3-section pattern

	var titleColor lipgloss.Color = successColor

	// Title section - manually centered using runewidth for emoji handling
	titleVisualWidth := lipgloss.Width(successTitle)
	leftPad := (modalWidth - titleVisualWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	rightPad := modalWidth - titleVisualWidth - leftPad
	centeredTitle := strings.Repeat(" ", leftPad) + successTitle + strings.Repeat(" ", rightPad)

	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(titleColor).
		Render(centeredTitle)

	// Message section (with top border)
	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Width(modalWidth).
		Render(strings.Join(messageLines, "\n"))

	// Footer section (with top border)
	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(footer)

	// Combine sections
	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}
