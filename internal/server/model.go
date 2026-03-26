package server

import (
	"time"

	"github.com/alexgorbatchev/agentation-cli/internal/protocol"
)

type UnixMilli = protocol.UnixMilli

type AnnotationIntent = protocol.AnnotationIntent

type AnnotationSeverity = protocol.AnnotationSeverity

type AnnotationStatus = protocol.AnnotationStatus

const (
	StatusPending      = protocol.StatusPending
	StatusAcknowledged = protocol.StatusAcknowledged
	StatusResolved     = protocol.StatusResolved
	StatusDismissed    = protocol.StatusDismissed
)

type Session = protocol.Session

type ThreadMessage = protocol.ThreadMessage

type Annotation = protocol.Annotation

type SessionWithAnnotations = protocol.SessionWithAnnotations

type ActionRequest = protocol.ActionRequest

type EventType = protocol.EventType

const (
	EventAnnotationCreated = protocol.EventAnnotationCreated
	EventAnnotationUpdated = protocol.EventAnnotationUpdated
	EventAnnotationDeleted = protocol.EventAnnotationDeleted
	EventSessionCreated    = protocol.EventSessionCreated
	EventSessionUpdated    = protocol.EventSessionUpdated
	EventSessionClosed     = protocol.EventSessionClosed
	EventThreadMessage     = protocol.EventThreadMessage
	EventActionRequested   = protocol.EventActionRequested
)

type Event = protocol.Event

type sessionCreateInput struct {
	URL       string `json:"url"`
	ProjectID string `json:"projectId,omitempty"`
}

type actionRequestInput struct {
	Output string `json:"output"`
}

type threadInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type pendingResponse = protocol.PendingResponse

type deliveredInfo = protocol.DeliveredInfo

type actionResponse = protocol.ActionResponse

func nowUnixMilli() UnixMilli {
	return UnixMilli(time.Now().UnixMilli())
}
