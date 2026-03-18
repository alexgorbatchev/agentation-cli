package store

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/router/model"
)

var (
	ErrNoMatchingSession = errors.New("no matching session")
	ErrAmbiguousSession  = errors.New("ambiguous session")
)

type RegisterOutcome string

const (
	RegisterOutcomeNew       RegisterOutcome = "new"
	RegisterOutcomeUpdated   RegisterOutcome = "updated"
	RegisterOutcomeHeartbeat RegisterOutcome = "heartbeat"
)

type Registry struct {
	mu              sync.RWMutex
	sessions        map[string]model.Session
	originToSession map[string]string
	staleAfter      time.Duration
	now             func() time.Time
}

func NewRegistry(staleAfter time.Duration) *Registry {
	return &Registry{
		sessions:        map[string]model.Session{},
		originToSession: map[string]string{},
		staleAfter:      staleAfter,
		now:             time.Now,
	}
}

func (r *Registry) Register(input model.RegisterRequest) model.Session {
	session, _ := r.RegisterWithOutcome(input)
	return session
}

func (r *Registry) RegisterWithOutcome(input model.RegisterRequest) (model.Session, RegisterOutcome) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.pruneStaleLocked()

	sessionID := strings.TrimSpace(input.SessionID)
	nextSession := model.Session{
		SessionID:   sessionID,
		ProjectID:   strings.TrimSpace(input.ProjectID),
		RepoID:      strings.TrimSpace(input.RepoID),
		Root:        cleanPath(input.Root),
		DisplayName: strings.TrimSpace(input.DisplayName),
		Endpoint:    strings.TrimSpace(input.Endpoint),
		LastSeenAt:  r.now(),
	}

	previousSession, existed := r.sessions[sessionID]
	r.sessions[sessionID] = nextSession

	if !existed {
		return nextSession, RegisterOutcomeNew
	}

	if previousSession.ProjectID == nextSession.ProjectID &&
		previousSession.RepoID == nextSession.RepoID &&
		previousSession.Root == nextSession.Root &&
		previousSession.DisplayName == nextSession.DisplayName &&
		previousSession.Endpoint == nextSession.Endpoint {
		return nextSession, RegisterOutcomeHeartbeat
	}

	return nextSession, RegisterOutcomeUpdated
}

func (r *Registry) Unregister(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.sessions, sessionID)
	for origin, mappedSessionID := range r.originToSession {
		if mappedSessionID == sessionID {
			delete(r.originToSession, origin)
		}
	}
}

func (r *Registry) List() []model.Session {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.pruneStaleLocked()

	result := make([]model.Session, 0, len(r.sessions))
	for _, session := range r.sessions {
		result = append(result, session)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].ProjectID == result[j].ProjectID {
			return result[i].LastSeenAt.After(result[j].LastSeenAt)
		}
		return result[i].ProjectID < result[j].ProjectID
	})

	return result
}

func (r *Registry) Resolve(projectID string, sourcePath string, origin string) (model.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.pruneStaleLocked()

	active := make([]model.Session, 0, len(r.sessions))
	for _, session := range r.sessions {
		active = append(active, session)
	}
	if len(active) == 0 {
		return model.Session{}, ErrNoMatchingSession
	}

	if projectID != "" {
		candidates := filterByProjectID(active, projectID)
		if len(candidates) == 0 {
			return model.Session{}, ErrNoMatchingSession
		}
		selected := newestSession(candidates)
		r.rememberOriginLocked(origin, selected.SessionID)
		return selected, nil
	}

	if origin != "" {
		if sessionID, ok := r.originToSession[origin]; ok {
			if session, found := r.sessions[sessionID]; found {
				return session, nil
			}
			delete(r.originToSession, origin)
		}
	}

	if sourcePath != "" {
		candidates := filterByPath(active, sourcePath)
		switch len(candidates) {
		case 0:
			// continue to fallback below
		case 1:
			r.rememberOriginLocked(origin, candidates[0].SessionID)
			return candidates[0], nil
		default:
			best := candidates[0]
			bestRootLen := len(best.Root)
			ambiguous := false
			for _, candidate := range candidates[1:] {
				candidateRootLen := len(candidate.Root)
				if candidateRootLen > bestRootLen {
					best = candidate
					bestRootLen = candidateRootLen
					ambiguous = false
					continue
				}
				if candidateRootLen == bestRootLen {
					ambiguous = true
				}
			}
			if ambiguous {
				return model.Session{}, ErrAmbiguousSession
			}
			r.rememberOriginLocked(origin, best.SessionID)
			return best, nil
		}
	}

	if len(active) == 1 {
		r.rememberOriginLocked(origin, active[0].SessionID)
		return active[0], nil
	}

	return model.Session{}, ErrAmbiguousSession
}

func (r *Registry) rememberOriginLocked(origin string, sessionID string) {
	if origin == "" {
		return
	}
	r.originToSession[origin] = sessionID
}

func (r *Registry) pruneStaleLocked() {
	if r.staleAfter <= 0 {
		return
	}

	cutoff := r.now().Add(-r.staleAfter)
	for sessionID, session := range r.sessions {
		if session.LastSeenAt.Before(cutoff) {
			delete(r.sessions, sessionID)
			for origin, mappedSessionID := range r.originToSession {
				if mappedSessionID == sessionID {
					delete(r.originToSession, origin)
				}
			}
		}
	}
}

func cleanPath(pathValue string) string {
	trimmed := strings.TrimSpace(pathValue)
	if trimmed == "" {
		return ""
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func filterByProjectID(sessions []model.Session, projectID string) []model.Session {
	result := make([]model.Session, 0)
	for _, session := range sessions {
		if session.ProjectID == projectID {
			result = append(result, session)
		}
	}
	return result
}

func filterByPath(sessions []model.Session, sourcePath string) []model.Session {
	result := make([]model.Session, 0)
	cleanSourcePath := cleanPath(sourcePath)
	if cleanSourcePath == "" {
		return result
	}
	for _, session := range sessions {
		if session.Root == "" {
			continue
		}
		if hasPathPrefix(cleanSourcePath, session.Root) {
			result = append(result, session)
		}
	}
	return result
}

func newestSession(sessions []model.Session) model.Session {
	if len(sessions) == 0 {
		return model.Session{}
	}
	selected := sessions[0]
	for _, session := range sessions[1:] {
		if session.LastSeenAt.After(selected.LastSeenAt) {
			selected = session
		}
	}
	return selected
}

func hasPathPrefix(sourcePath string, rootPath string) bool {
	normalizedSource := filepath.ToSlash(cleanPath(sourcePath))
	normalizedRoot := filepath.ToSlash(cleanPath(rootPath))
	if normalizedSource == "" || normalizedRoot == "" {
		return false
	}
	if normalizedSource == normalizedRoot {
		return true
	}
	return strings.HasPrefix(normalizedSource, normalizedRoot+"/")
}
