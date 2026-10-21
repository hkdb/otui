package model

import "context"

// A request is one user message and everything it triggers: the provider
// call, tool calls and the follow-up steps. Each request has an ID and a
// context. Esc cancels the context, and results of requests that are no
// longer current are ignored by the UI.
//
// These fields are only used on the UI goroutine. Commands capture the
// request's context and ID when they're created.

// RequestID returns the ID of the current request. Messages from Cmds carry
// the ID of the request they belong to.
func (m *Model) RequestID() uint64 {
	return m.requestID
}

// beginRequest starts a new request, cancelling the previous one
func (m *Model) beginRequest() (context.Context, uint64) {
	m.CancelRequest()
	m.requestCtx, m.requestCancel = context.WithCancel(context.Background())
	return m.requestCtx, m.requestID
}

// CancelRequest cancels the current request (provider call and tool calls)
// and makes its results stale
func (m *Model) CancelRequest() {
	if m.requestCancel != nil {
		m.requestCancel()
	}
	m.requestCtx, m.requestCancel = nil, nil
	m.requestID++
}

// requestContext returns the context of request id, or nil if id is no
// longer the current request
func (m *Model) requestContext(id uint64) context.Context {
	if id != m.requestID {
		return nil
	}
	if m.requestCtx == nil {
		return context.Background()
	}
	return m.requestCtx
}
