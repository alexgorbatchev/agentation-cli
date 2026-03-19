package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Store struct {
	mu sync.RWMutex

	sessions    map[string]Session
	annotations map[string]Annotation
	events      map[string][]Event

	sequence int64

	subsMu      sync.RWMutex
	nextSubID   int64
	globalSubs  map[int64]chan Event
	sessionSubs map[string]map[int64]chan Event

	idCounter uint64

	persistence persistenceBackend
}

func NewStore() *Store {
	store := &Store{
		sessions:    make(map[string]Session),
		annotations: make(map[string]Annotation),
		events:      make(map[string][]Event),
		globalSubs:  make(map[int64]chan Event),
		sessionSubs: make(map[string]map[int64]chan Event),
	}

	backend, err := newPersistenceBackend()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Store] SQLite unavailable, using in-memory store: %v\n", err)
		return store
	}
	if backend == nil {
		fmt.Fprintln(os.Stderr, "[Store] Using in-memory store (AGENTATION_STORE=memory)")
		return store
	}

	snapshot, err := backend.LoadSnapshot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Store] Failed to load SQLite snapshot, using in-memory store: %v\n", err)
		_ = backend.Close()
		return store
	}

	store.sessions = snapshot.Sessions
	store.annotations = snapshot.Annotations
	store.events = snapshot.Events
	store.sequence = snapshot.Sequence
	store.persistence = backend

	fmt.Fprintln(os.Stderr, "[Store] Using SQLite store")
	return store
}

func (s *Store) CreateSession(url, projectID string) Session {
	s.mu.Lock()

	session := Session{
		ID:        s.newID(),
		URL:       url,
		Status:    "active",
		CreatedAt: nowISO(),
		ProjectID: projectID,
	}
	s.sessions[session.ID] = session
	s.persistSessionLocked(session)
	event := s.emitLocked(EventSessionCreated, session.ID, session)

	s.mu.Unlock()
	s.publish(event)
	return session
}

func (s *Store) ListSessions() []Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

func (s *Store) GetSession(id string) (Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[id]
	return session, ok
}

func (s *Store) GetSessionWithAnnotations(id string) (SessionWithAnnotations, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return SessionWithAnnotations{}, false
	}

	annotations := make([]Annotation, 0)
	for _, annotation := range s.annotations {
		if annotation.SessionID == id {
			annotations = append(annotations, annotation)
		}
	}

	return SessionWithAnnotations{
		Session:     session,
		Annotations: annotations,
	}, true
}

func (s *Store) AddAnnotation(sessionID string, annotation Annotation) (Annotation, bool) {
	s.mu.Lock()

	if _, ok := s.sessions[sessionID]; !ok {
		s.mu.Unlock()
		return Annotation{}, false
	}

	annotation.ID = s.newID()
	annotation.SessionID = sessionID
	annotation.Status = StatusPending
	annotation.CreatedAt = nowISO()
	if annotation.Timestamp == 0 {
		annotation.Timestamp = time.Now().UnixMilli()
	}
	if annotation.Thread == nil {
		annotation.Thread = []ThreadMessage{}
	}

	s.annotations[annotation.ID] = annotation
	s.persistAnnotationLocked(annotation)
	event := s.emitLocked(EventAnnotationCreated, sessionID, annotation)

	s.mu.Unlock()
	s.publish(event)
	return annotation, true
}

func (s *Store) GetAnnotation(id string) (Annotation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	annotation, ok := s.annotations[id]
	return annotation, ok
}

func (s *Store) UpdateAnnotation(id string, patch map[string]any) (Annotation, bool) {
	s.mu.Lock()

	annotation, ok := s.annotations[id]
	if !ok {
		s.mu.Unlock()
		return Annotation{}, false
	}

	if value, exists := patch["comment"].(string); exists {
		annotation.Comment = value
	}
	if value, exists := patch["status"].(string); exists {
		annotation.Status = AnnotationStatus(value)
	}
	if value, exists := patch["resolvedBy"].(string); exists {
		annotation.ResolvedBy = value
	}
	if value, exists := patch["resolvedAt"].(string); exists {
		annotation.ResolvedAt = value
	}

	if annotation.Status == StatusResolved || annotation.Status == StatusDismissed {
		if annotation.ResolvedAt == "" {
			annotation.ResolvedAt = nowISO()
		}
		if annotation.ResolvedBy == "" {
			annotation.ResolvedBy = "agent"
		}
	}

	annotation.UpdatedAt = nowISO()
	s.annotations[id] = annotation
	s.persistAnnotationLocked(annotation)
	event := s.emitLocked(EventAnnotationUpdated, annotation.SessionID, annotation)

	s.mu.Unlock()
	s.publish(event)
	return annotation, true
}

func (s *Store) DeleteAnnotation(id string) (Annotation, bool) {
	s.mu.Lock()

	annotation, ok := s.annotations[id]
	if !ok {
		s.mu.Unlock()
		return Annotation{}, false
	}

	delete(s.annotations, id)
	s.deleteAnnotationLocked(id)
	event := s.emitLocked(EventAnnotationDeleted, annotation.SessionID, annotation)

	s.mu.Unlock()
	s.publish(event)
	return annotation, true
}

func (s *Store) AddThreadMessage(annotationID, role, content string) (Annotation, bool) {
	s.mu.Lock()

	annotation, ok := s.annotations[annotationID]
	if !ok {
		s.mu.Unlock()
		return Annotation{}, false
	}

	message := ThreadMessage{
		ID:        s.newID(),
		Role:      role,
		Content:   content,
		Timestamp: time.Now().UnixMilli(),
	}
	annotation.Thread = append(annotation.Thread, message)
	annotation.UpdatedAt = nowISO()
	s.annotations[annotationID] = annotation
	s.persistAnnotationLocked(annotation)
	event := s.emitLocked(EventThreadMessage, annotation.SessionID, annotation)

	s.mu.Unlock()
	s.publish(event)
	return annotation, true
}

func (s *Store) GetAnnotationsNeedingAttention(sessionID string) []Annotation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	annotations := make([]Annotation, 0)
	for _, annotation := range s.annotations {
		if annotation.SessionID != sessionID {
			continue
		}
		if needsAttention(annotation) {
			annotations = append(annotations, annotation)
		}
	}
	return annotations
}

func (s *Store) GetAllAnnotationsNeedingAttention() []Annotation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	annotations := make([]Annotation, 0)
	for _, annotation := range s.annotations {
		if needsAttention(annotation) {
			annotations = append(annotations, annotation)
		}
	}
	return annotations
}

func (s *Store) GetAllAnnotationsNeedingAttentionByProjectID(projectID string) []Annotation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	trimmedProjectID := strings.TrimSpace(projectID)
	if trimmedProjectID == "" {
		return []Annotation{}
	}

	sessionIDs := make(map[string]struct{})
	for _, session := range s.sessions {
		if session.ProjectID == trimmedProjectID {
			sessionIDs[session.ID] = struct{}{}
		}
	}

	annotations := make([]Annotation, 0)
	for _, annotation := range s.annotations {
		if _, ok := sessionIDs[annotation.SessionID]; !ok {
			continue
		}
		if needsAttention(annotation) {
			annotations = append(annotations, annotation)
		}
	}
	return annotations
}

func (s *Store) GetSessionAnnotations(sessionID string) []Annotation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	annotations := make([]Annotation, 0)
	for _, annotation := range s.annotations {
		if annotation.SessionID == sessionID {
			annotations = append(annotations, annotation)
		}
	}
	return annotations
}

func (s *Store) GetEventsSince(sessionID string, sequence int64) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := s.events[sessionID]
	filtered := make([]Event, 0)
	for _, event := range events {
		if event.Sequence > sequence {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func (s *Store) EmitActionRequested(sessionID string, request ActionRequest) {
	s.mu.Lock()
	event := s.emitLocked(EventActionRequested, sessionID, request)
	s.mu.Unlock()

	s.publish(event)
}

func (s *Store) SubscribeAll() (<-chan Event, func()) {
	id := atomic.AddInt64(&s.nextSubID, 1)
	ch := make(chan Event, 64)

	s.subsMu.Lock()
	s.globalSubs[id] = ch
	s.subsMu.Unlock()

	return ch, func() {
		s.subsMu.Lock()
		defer s.subsMu.Unlock()
		delete(s.globalSubs, id)
	}
}

func (s *Store) SubscribeSession(sessionID string) (<-chan Event, func()) {
	id := atomic.AddInt64(&s.nextSubID, 1)
	ch := make(chan Event, 64)

	s.subsMu.Lock()
	if s.sessionSubs[sessionID] == nil {
		s.sessionSubs[sessionID] = make(map[int64]chan Event)
	}
	s.sessionSubs[sessionID][id] = ch
	s.subsMu.Unlock()

	return ch, func() {
		s.subsMu.Lock()
		defer s.subsMu.Unlock()
		subs := s.sessionSubs[sessionID]
		if subs == nil {
			return
		}
		delete(subs, id)
		if len(subs) == 0 {
			delete(s.sessionSubs, sessionID)
		}
	}
}

func (s *Store) emitLocked(kind EventType, sessionID string, payload any) Event {
	s.sequence++
	event := Event{
		Type:      kind,
		Timestamp: nowISO(),
		SessionID: sessionID,
		Sequence:  s.sequence,
		Payload:   payload,
	}
	s.events[sessionID] = append(s.events[sessionID], event)
	s.persistEventLocked(event)
	return event
}

func (s *Store) publish(event Event) {
	s.subsMu.RLock()
	global := make([]chan Event, 0, len(s.globalSubs))
	for _, ch := range s.globalSubs {
		global = append(global, ch)
	}
	session := make([]chan Event, 0)
	if subs := s.sessionSubs[event.SessionID]; subs != nil {
		session = make([]chan Event, 0, len(subs))
		for _, ch := range subs {
			session = append(session, ch)
		}
	}
	s.subsMu.RUnlock()

	for _, ch := range global {
		sendWithBackpressure(ch, event)
	}
	for _, ch := range session {
		sendWithBackpressure(ch, event)
	}
}

func (s *Store) persistSessionLocked(session Session) {
	if s.persistence == nil {
		return
	}
	if err := s.persistence.UpsertSession(session); err != nil {
		fmt.Fprintf(os.Stderr, "[Store] Failed to persist session: %v\n", err)
	}
}

func (s *Store) persistAnnotationLocked(annotation Annotation) {
	if s.persistence == nil {
		return
	}
	if err := s.persistence.UpsertAnnotation(annotation); err != nil {
		fmt.Fprintf(os.Stderr, "[Store] Failed to persist annotation: %v\n", err)
	}
}

func (s *Store) deleteAnnotationLocked(annotationID string) {
	if s.persistence == nil {
		return
	}
	if err := s.persistence.DeleteAnnotation(annotationID); err != nil {
		fmt.Fprintf(os.Stderr, "[Store] Failed to delete annotation from SQLite: %v\n", err)
	}
}

func (s *Store) persistEventLocked(event Event) {
	if s.persistence == nil {
		return
	}
	if err := s.persistence.InsertEvent(event); err != nil {
		fmt.Fprintf(os.Stderr, "[Store] Failed to persist event: %v\n", err)
	}
}

func sendWithBackpressure(ch chan Event, event Event) {
	ch <- event
}

func needsAttention(annotation Annotation) bool {
	if annotation.Status == "" || annotation.Status == StatusPending {
		return true
	}

	if len(annotation.Thread) == 0 {
		return false
	}

	last := annotation.Thread[len(annotation.Thread)-1]
	return last.Role == "human"
}

func (s *Store) newID() string {
	counter := atomic.AddUint64(&s.idCounter, 1)
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d-%d", time.Now().UnixMilli(), counter)
	}
	return fmt.Sprintf("%d-%s", time.Now().UnixMilli(), hex.EncodeToString(bytes))
}
