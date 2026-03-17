package server

import "time"

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
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt,omitempty"`
	ProjectID string         `json:"projectId,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type ThreadMessage struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

type Annotation struct {
	ID                   string             `json:"id"`
	SessionID            string             `json:"sessionId,omitempty"`
	X                    float64            `json:"x,omitempty"`
	Y                    float64            `json:"y,omitempty"`
	Comment              string             `json:"comment"`
	Element              string             `json:"element"`
	ElementPath          string             `json:"elementPath"`
	Timestamp            int64              `json:"timestamp,omitempty"`
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
	CreatedAt            string             `json:"createdAt,omitempty"`
	UpdatedAt            string             `json:"updatedAt,omitempty"`
	ResolvedAt           string             `json:"resolvedAt,omitempty"`
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
	RequestedAt string       `json:"timestamp"`
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
	Timestamp string    `json:"timestamp"`
	SessionID string    `json:"sessionId"`
	Sequence  int64     `json:"sequence"`
	Payload   any       `json:"payload"`
}

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

type pendingResponse struct {
	Count       int          `json:"count"`
	Annotations []Annotation `json:"annotations"`
}

type deliveredInfo struct {
	SSEListeners int `json:"sseListeners"`
	Webhooks     int `json:"webhooks"`
	Total        int `json:"total"`
}

type actionResponse struct {
	Success         bool          `json:"success"`
	AnnotationCount int           `json:"annotationCount"`
	Delivered       deliveredInfo `json:"delivered"`
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
