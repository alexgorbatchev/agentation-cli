package protocol

type AnnotationIntent string

type AnnotationSeverity string

type AnnotationStatus string

const (
	StatusPending      AnnotationStatus = "pending"
	StatusAcknowledged AnnotationStatus = "acknowledged"
	StatusResolved     AnnotationStatus = "resolved"
	StatusDismissed    AnnotationStatus = "dismissed"
)

type Session struct {
	ID        string         `json:"id"`
	URL       string         `json:"url"`
	Status    string         `json:"status"`
	CreatedAt UnixMilli      `json:"createdAt"`
	UpdatedAt UnixMilli      `json:"updatedAt,omitempty"`
	ProjectID string         `json:"projectId,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type ThreadMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp UnixMilli `json:"timestamp"`
}

type Annotation struct {
	ID                   string             `json:"id"`
	SessionID            string             `json:"sessionId,omitempty"`
	X                    float64            `json:"x,omitempty"`
	Y                    float64            `json:"y,omitempty"`
	Comment              string             `json:"comment"`
	Element              string             `json:"element"`
	ElementPath          string             `json:"elementPath"`
	Timestamp            UnixMilli          `json:"timestamp,omitempty"`
	SelectedText         string             `json:"selectedText,omitempty"`
	BoundingBox          map[string]any     `json:"boundingBox,omitempty"`
	NearbyText           string             `json:"nearbyText,omitempty"`
	CSSClasses           string             `json:"cssClasses,omitempty"`
	NearbyElements       string             `json:"nearbyElements,omitempty"`
	ComputedStyles       string             `json:"computedStyles,omitempty"`
	FullPath             string             `json:"fullPath,omitempty"`
	Accessibility        string             `json:"accessibility,omitempty"`
	IsMultiSelect        bool               `json:"isMultiSelect,omitempty"`
	IsFixed              bool               `json:"isFixed,omitempty"`
	ReactComponents      string             `json:"reactComponents,omitempty"`
	SourceFile           string             `json:"sourceFile,omitempty"`
	ElementBoundingBoxes []map[string]any   `json:"elementBoundingBoxes,omitempty"`
	URL                  string             `json:"url,omitempty"`
	Intent               AnnotationIntent   `json:"intent,omitempty"`
	Severity             AnnotationSeverity `json:"severity,omitempty"`
	Status               AnnotationStatus   `json:"status,omitempty"`
	Thread               []ThreadMessage    `json:"thread,omitempty"`
	CreatedAt            UnixMilli          `json:"createdAt,omitempty"`
	UpdatedAt            UnixMilli          `json:"updatedAt,omitempty"`
	ResolvedAt           UnixMilli          `json:"resolvedAt,omitempty"`
	ResolvedBy           string             `json:"resolvedBy,omitempty"`
	AuthorID             string             `json:"authorId,omitempty"`
}

type SessionWithAnnotations struct {
	Session
	Annotations []Annotation `json:"annotations"`
}

type ActionRequest struct {
	SessionID   string       `json:"sessionId"`
	Annotations []Annotation `json:"annotations"`
	Output      string       `json:"output"`
	RequestedAt UnixMilli    `json:"timestamp"`
}

type EventType string

const (
	EventAnnotationCreated EventType = "annotation.created"
	EventAnnotationUpdated EventType = "annotation.updated"
	EventAnnotationDeleted EventType = "annotation.deleted"
	EventSessionCreated    EventType = "session.created"
	EventSessionUpdated    EventType = "session.updated"
	EventSessionClosed     EventType = "session.closed"
	EventThreadMessage     EventType = "thread.message"
	EventActionRequested   EventType = "action.requested"
)

type Event struct {
	Type      EventType `json:"type"`
	Timestamp UnixMilli `json:"timestamp"`
	SessionID string    `json:"sessionId"`
	Sequence  int64     `json:"sequence"`
	Payload   any       `json:"payload"`
}

type PendingResponse struct {
	Count       int          `json:"count"`
	Annotations []Annotation `json:"annotations"`
}

type DeliveredInfo struct {
	SSEListeners int `json:"sseListeners"`
	Webhooks     int `json:"webhooks"`
	Total        int `json:"total"`
}

type ActionResponse struct {
	Success         bool          `json:"success"`
	AnnotationCount int           `json:"annotationCount"`
	Delivered       DeliveredInfo `json:"delivered"`
}
