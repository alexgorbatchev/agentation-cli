package api

import "time"

type Session struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
}

type ThreadMessage struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

type Annotation struct {
	ID              string          `json:"id"`
	SessionID       string          `json:"sessionId,omitempty"`
	Comment         string          `json:"comment"`
	Element         string          `json:"element"`
	ElementPath     string          `json:"elementPath"`
	URL             string          `json:"url,omitempty"`
	Intent          string          `json:"intent,omitempty"`
	Severity        string          `json:"severity,omitempty"`
	Status          string          `json:"status,omitempty"`
	Timestamp       int64           `json:"timestamp,omitempty"`
	NearbyText      string          `json:"nearbyText,omitempty"`
	ReactComponents string          `json:"reactComponents,omitempty"`
	CreatedAt       string          `json:"createdAt,omitempty"`
	Thread          []ThreadMessage `json:"thread,omitempty"`
}

type SessionWithAnnotations struct {
	Session
	Annotations []Annotation `json:"annotations"`
}

type PendingResponse struct {
	Count       int          `json:"count"`
	Annotations []Annotation `json:"annotations"`
}

type WatchOptions struct {
	SessionID   string
	ProjectID   string
	BatchWindow time.Duration
	Timeout     time.Duration
}

type WatchOutput struct {
	Timeout     bool         `json:"timeout"`
	Message     string       `json:"message,omitempty"`
	Count       int          `json:"count,omitempty"`
	Sessions    []string     `json:"sessions,omitempty"`
	Annotations []Annotation `json:"annotations,omitempty"`
}
