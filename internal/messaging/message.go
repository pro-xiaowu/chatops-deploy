package messaging

import (
	"context"
	"errors"

	"chatops-deploy/internal/domain"
)

var (
	ErrUnsupportedDecode = errors.New("message provider does not decode external messages")
	ErrUnsupportedSend   = errors.New("message provider does not send messages")
)

type Capabilities struct {
	Provider   domain.MessageProvider `json:"provider"`
	Configured bool                   `json:"configured"`
	Healthy    bool                   `json:"healthy"`
	Reason     string                 `json:"reason,omitempty"`
}

type Request struct {
	Method  string
	Headers map[string]string
	Body    []byte
}

type Incoming struct {
	Provider       domain.MessageProvider
	EventID        string
	ConversationID string
	SubjectID      string
	DisplayName    string
	Command        string
	Arguments      []string
	OperationID    string
	Decision       string
	Challenge      string
}

type Notification struct {
	OperationID  string
	Destination  string
	Text         string
	ApprovalCard map[string]any
}

type Provider interface {
	Name() domain.MessageProvider
	Capabilities(context.Context) Capabilities
	Decode(context.Context, Request) (Incoming, error)
	Send(context.Context, Notification) error
	Check(context.Context) error
}
