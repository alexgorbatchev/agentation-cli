package model

import "time"

type Session struct {
	SessionID   string    `json:"sessionId"`
	ProjectID   string    `json:"projectId"`
	RepoID      string    `json:"repoId,omitempty"`
	Root        string    `json:"root"`
	DisplayName string    `json:"displayName"`
	Endpoint    string    `json:"endpoint"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
}

type RegisterRequest struct {
	SessionID   string `json:"sessionId"`
	ProjectID   string `json:"projectId"`
	RepoID      string `json:"repoId,omitempty"`
	Root        string `json:"root"`
	DisplayName string `json:"displayName"`
	Endpoint    string `json:"endpoint"`
}

type UnregisterRequest struct {
	SessionID string `json:"sessionId"`
}

type OpenRequest struct {
	ProjectID string `json:"projectId"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Origin    string `json:"origin"`
}

type PingRequest struct {
	ProjectID string `json:"projectId"`
	Origin    string `json:"origin"`
}
