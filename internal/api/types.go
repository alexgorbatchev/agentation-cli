package api

import (
	"time"

	"github.com/alexgorbatchev/agentation-cli/internal/protocol"
)

type Session = protocol.Session

type ThreadMessage = protocol.ThreadMessage

type Annotation = protocol.Annotation

type SessionWithAnnotations = protocol.SessionWithAnnotations

type PendingResponse = protocol.PendingResponse

type WatchOptions struct {
	SessionID string
	ProjectID string
	Timeout   time.Duration
}

type WatchOutput struct {
	Timeout     bool         `json:"timeout"`
	Message     string       `json:"message,omitempty"`
	Count       int          `json:"count,omitempty"`
	Sessions    []string     `json:"sessions,omitempty"`
	Annotations []Annotation `json:"annotations,omitempty"`
}
