package ui

import (
	"errors"
	"strings"
	"testing"

	appmodel "otui/model"
)

// cancelledAppView returns an AppView whose request was cancelled with Esc,
// and the ID of that request
func cancelledAppView() (AppView, uint64) {
	a := AppView{dataModel: &appmodel.Model{}, currentResp: &strings.Builder{}}
	old := a.dataModel.RequestID()
	a.dataModel.Streaming = true
	a.currentResp.WriteString("partial")
	a.executingTool = "x"
	a.cancelRequest()
	return a, old
}

func TestEscResetsRequestState(t *testing.T) {
	a, old := cancelledAppView()
	if a.dataModel.Streaming || a.executingTool != "" {
		t.Errorf("state not reset: streaming=%v executingTool=%q", a.dataModel.Streaming, a.executingTool)
	}
	if a.dataModel.RequestID() == old {
		t.Error("request not cancelled")
	}
}

func TestStaleRequestMessagesIgnored(t *testing.T) {
	a, old := cancelledAppView()
	// A new question was asked after Esc
	a.dataModel.Streaming = true
	a.currentResp.Reset()

	msgs := []any{
		toolCallsDetectedMsg{ToolCalls: []ToolCall{{Name: "x.y"}}, RequestID: old},
		toolPermissionRequestMsg{ToolName: "x.y", RequestID: old},
		toolResultsReadyMsg{RequestID: old},
		toolExecutionCompleteMsg{FullResponse: "old answer", RequestID: old},
		toolExecutionErrorMsg{Err: errors.New("context canceled"), RequestID: old},
	}
	for _, msg := range msgs {
		b, cmd := a.handleToolMessage(msg)
		if cmd != nil || len(b.dataModel.Messages) != 0 || b.executingTool != "" || b.waitingForPermission || !b.dataModel.Streaming {
			t.Errorf("%T of a cancelled request was acted on", msg)
		}
	}

	streaming := []any{
		streamChunkMsg{Chunk: "old answer", RequestID: old},
		streamDoneMsg{FullResponse: "old answer", RequestID: old},
		streamErrorMsg{Err: errors.New("context canceled"), RequestID: old},
	}
	for _, msg := range streaming {
		b, cmd := a.handleStreamingMessage(msg)
		if cmd != nil || len(b.dataModel.Messages) != 0 || b.currentResp.Len() != 0 || !b.dataModel.Streaming {
			t.Errorf("%T of a cancelled request was acted on", msg)
		}
	}
}
