package ui

import (
	"otui/model"
)

// Message type aliases for backward compatibility
type Message = model.Message

// Message type aliases - these are now defined in model package
type streamChunkMsg = model.StreamChunkMsg
type streamDoneMsg = model.StreamDoneMsg
type streamErrorMsg = model.StreamErrorMsg
type streamChunksCollectedMsg = model.StreamChunksCollectedMsg
type displayChunkTickMsg = model.DisplayChunkTickMsg
type toolCallsDetectedMsg = model.ToolCallsDetectedMsg
type toolExecutionCompleteMsg = model.ToolExecutionCompleteMsg
type toolExecutionErrorMsg = model.ToolExecutionErrorMsg
type markdownRenderedMsg = model.MarkdownRenderedMsg
type modelsListMsg = model.ModelsListMsg
type sessionsListMsg = model.SessionsListMsg
type sessionLoadedMsg = model.SessionLoadedMsg
type sessionSavedMsg = model.SessionSavedMsg
type sessionRenamedMsg = model.SessionRenamedMsg
type sessionExportedMsg = model.SessionExportedMsg
type sessionImportedMsg = model.SessionImportedMsg
type exportCleanupDoneMsg = model.ExportCleanupDoneMsg
type dataExportedMsg = model.DataExportedMsg
type dataExportCleanupDoneMsg = model.DataExportCleanupDoneMsg
type flashTickMsg = model.FlashTickMsg
type pluginOperationCompleteMsg = model.PluginOperationCompleteMsg
type pluginStartupCompleteMsg = model.PluginStartupCompleteMsg
type registryRefreshCompleteMsg = model.RegistryRefreshCompleteMsg
type editorContentMsg = model.EditorContentMsg
type editorErrorMsg = model.EditorErrorMsg

type SettingFieldType int

const (
	SettingTypeDataDir SettingFieldType = iota
	SettingTypeProviderLink
	SettingTypeModel
	SettingTypeSystemPrompt
	SettingTypePluginsEnabled
)

type SettingFieldValidation int

const (
	FieldValidationNone SettingFieldValidation = iota
	FieldValidationPending
	FieldValidationSuccess
	FieldValidationError
)

type SettingField struct {
	Label        string
	Value        string
	DefaultValue string
	Type         SettingFieldType
	Validation   SettingFieldValidation
	ErrorMsg     string
}
