package server

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCoreFlows(t *testing.T) {
	t.Setenv("AGENTATION_STORE", "memory")
	store := NewStore()

	if _, ok := store.GetSession("missing"); ok {
		t.Fatal("GetSession should return missing for unknown id")
	}
	if _, ok := store.GetSessionWithAnnotations("missing"); ok {
		t.Fatal("GetSessionWithAnnotations should return missing for unknown id")
	}

	session := store.CreateSession("http://example.com/page", "p1")
	if session.ID == "" {
		t.Fatal("session id should not be empty")
	}

	sessions := store.ListSessions()
	if len(sessions) != 1 {
		t.Fatalf("ListSessions length = %d, want 1", len(sessions))
	}

	if _, ok := store.AddAnnotation("missing", Annotation{Comment: "x", Element: "button", ElementPath: "body > button"}); ok {
		t.Fatal("AddAnnotation should fail for unknown session")
	}

	annotation, ok := store.AddAnnotation(session.ID, Annotation{
		Comment:     "Fix button",
		Element:     "button",
		ElementPath: "body > button",
	})
	if !ok {
		t.Fatal("AddAnnotation should succeed")
	}
	if annotation.ID == "" || annotation.Status != StatusPending {
		t.Fatalf("unexpected annotation: %#v", annotation)
	}
	if annotation.Timestamp == 0 {
		t.Fatal("annotation timestamp should be set")
	}
	if annotation.Thread == nil {
		t.Fatal("annotation thread slice should be initialized")
	}

	if _, ok := store.GetAnnotation(annotation.ID); !ok {
		t.Fatal("GetAnnotation should find inserted annotation")
	}
	if _, ok := store.GetAnnotation("missing"); ok {
		t.Fatal("GetAnnotation should not find unknown annotation")
	}

	if _, ok := store.UpdateAnnotation("missing", map[string]any{"comment": "x"}); ok {
		t.Fatal("UpdateAnnotation should fail for unknown id")
	}

	resolved, ok := store.UpdateAnnotation(annotation.ID, map[string]any{"status": "resolved"})
	if !ok {
		t.Fatal("UpdateAnnotation should succeed")
	}
	if resolved.ResolvedAt == "" || resolved.ResolvedBy == "" {
		t.Fatalf("resolved annotation should set resolution metadata: %#v", resolved)
	}

	updated, ok := store.UpdateAnnotation(annotation.ID, map[string]any{"comment": "Updated", "resolvedBy": "human", "resolvedAt": "custom-time"})
	if !ok {
		t.Fatal("UpdateAnnotation second patch should succeed")
	}
	if updated.Comment != "Updated" || updated.ResolvedBy != "human" || updated.ResolvedAt != "custom-time" {
		t.Fatalf("unexpected updated annotation: %#v", updated)
	}

	if _, ok := store.AddThreadMessage("missing", "human", "hello"); ok {
		t.Fatal("AddThreadMessage should fail for unknown annotation")
	}
	threaded, ok := store.AddThreadMessage(annotation.ID, "human", "Please also fix hover")
	if !ok {
		t.Fatal("AddThreadMessage should succeed")
	}
	if len(threaded.Thread) == 0 {
		t.Fatal("thread message should be appended")
	}

	withAnn, ok := store.GetSessionWithAnnotations(session.ID)
	if !ok || len(withAnn.Annotations) == 0 {
		t.Fatalf("expected session with annotations, got ok=%v len=%d", ok, len(withAnn.Annotations))
	}

	attention := store.GetAnnotationsNeedingAttention(session.ID)
	if len(attention) == 0 {
		t.Fatal("expected annotation to need attention after human thread reply")
	}

	allAttention := store.GetAllAnnotationsNeedingAttention()
	if len(allAttention) == 0 {
		t.Fatal("GetAllAnnotationsNeedingAttention should include annotation")
	}

	sessionAnnotations := store.GetSessionAnnotations(session.ID)
	if len(sessionAnnotations) == 0 {
		t.Fatal("GetSessionAnnotations should return inserted annotation")
	}

	eventsSinceZero := store.GetEventsSince(session.ID, 0)
	if len(eventsSinceZero) == 0 {
		t.Fatal("GetEventsSince should return session events")
	}

	store.EmitActionRequested(session.ID, ActionRequest{SessionID: session.ID, Output: "do work"})
	eventsSinceOne := store.GetEventsSince(session.ID, 1)
	if len(eventsSinceOne) == 0 {
		t.Fatal("EmitActionRequested should append event")
	}

	if _, ok := store.DeleteAnnotation("missing"); ok {
		t.Fatal("DeleteAnnotation should fail for unknown id")
	}
	deleted, ok := store.DeleteAnnotation(annotation.ID)
	if !ok || deleted.ID != annotation.ID {
		t.Fatal("DeleteAnnotation should remove existing annotation")
	}
}

func TestStoreSubscriptionsAndHelpers(t *testing.T) {
	t.Setenv("AGENTATION_STORE", "memory")
	store := NewStore()
	session := store.CreateSession("http://example.com", "")

	allCh, unsubAll := store.SubscribeAll()
	sessionCh, unsubSession := store.SubscribeSession(session.ID)

	_, ok := store.AddAnnotation(session.ID, Annotation{Comment: "A", Element: "button", ElementPath: "body > button"})
	if !ok {
		t.Fatal("AddAnnotation should succeed")
	}

	select {
	case <-allCh:
	default:
		t.Fatal("global subscriber should receive event")
	}
	select {
	case <-sessionCh:
	default:
		t.Fatal("session subscriber should receive event")
	}

	unsubAll()
	unsubSession()

	full := make(chan Event, 1)
	full <- Event{Type: EventAnnotationCreated}
	blocked := make(chan struct{})
	go func() {
		sendWithBackpressure(full, Event{Type: EventAnnotationUpdated})
		close(blocked)
	}()

	select {
	case <-blocked:
		t.Fatal("sendWithBackpressure should block while channel is full")
	case <-time.After(25 * time.Millisecond):
	}

	<-full
	select {
	case <-blocked:
	case <-time.After(1 * time.Second):
		t.Fatal("sendWithBackpressure should unblock after capacity is available")
	}

	if !needsAttention(Annotation{Status: ""}) {
		t.Fatal("empty status should need attention")
	}
	if !needsAttention(Annotation{Status: StatusPending}) {
		t.Fatal("pending should need attention")
	}
	if needsAttention(Annotation{Status: StatusAcknowledged}) {
		t.Fatal("acknowledged without thread should not need attention")
	}
	if !needsAttention(Annotation{Status: StatusAcknowledged, Thread: []ThreadMessage{{Role: "human"}}}) {
		t.Fatal("human last thread message should need attention")
	}
	if needsAttention(Annotation{Status: StatusAcknowledged, Thread: []ThreadMessage{{Role: "agent"}}}) {
		t.Fatal("agent last thread message should not need attention")
	}

	if id := store.newID(); id == "" {
		t.Fatal("newID should return non-empty id")
	}
}

func TestStoreBackpressurePublishDoesNotHoldStoreMutex(t *testing.T) {
	t.Setenv("AGENTATION_STORE", "memory")
	store := NewStore()
	session := store.CreateSession("http://example.com", "")

	baselineEventCount := len(store.GetEventsSince(session.ID, 0))
	events, unsubscribe := store.SubscribeSession(session.ID)
	defer unsubscribe()

	for i := 0; i < cap(events); i++ {
		_, ok := store.AddAnnotation(session.ID, Annotation{Comment: fmt.Sprintf("fill-%d", i), Element: "button", ElementPath: "body > button"})
		if !ok {
			t.Fatal("AddAnnotation should succeed while filling subscriber buffer")
		}
	}
	if len(events) != cap(events) {
		t.Fatalf("subscriber channel should be full; len=%d cap=%d", len(events), cap(events))
	}

	publishDone := make(chan struct{})
	publishErr := make(chan string, 1)
	go func() {
		_, ok := store.AddAnnotation(session.ID, Annotation{Comment: "blocked", Element: "button", ElementPath: "body > button"})
		if !ok {
			publishErr <- "AddAnnotation should succeed for blocked publish"
		}
		close(publishDone)
	}()

	select {
	case <-publishDone:
		t.Fatal("expected publish to block when subscriber channel is full")
	case <-time.After(25 * time.Millisecond):
	}

	readDone := make(chan int, 1)
	go func() {
		readDone <- len(store.GetEventsSince(session.ID, 0))
	}()

	select {
	case count := <-readDone:
		if want := baselineEventCount + cap(events) + 1; count != want {
			t.Fatalf("expected blocked event to remain durable before publish unblocks; got %d want %d", count, want)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("GetEventsSince blocked while publish was backpressured")
	}

	createDone := make(chan struct{})
	go func() {
		_ = store.CreateSession("http://example.com/other", "")
		close(createDone)
	}()

	select {
	case <-createDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("CreateSession blocked behind backpressured publish")
	}

	select {
	case <-events:
	case <-time.After(1 * time.Second):
		t.Fatal("expected buffered event to be available for draining")
	}

	select {
	case <-publishDone:
	case <-time.After(1 * time.Second):
		t.Fatal("blocked publish did not resume after drain")
	}

	select {
	case msg := <-publishErr:
		t.Fatal(msg)
	default:
	}
}

func TestStoreSQLitePersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.db")
	t.Setenv("AGENTATION_STORE", "sqlite")
	t.Setenv("AGENTATION_DB_PATH", dbPath)

	store := NewStore()
	session := store.CreateSession("http://example.com/persisted", "project-1")
	annotation, ok := store.AddAnnotation(session.ID, Annotation{Comment: "Persist me", Element: "button", ElementPath: "body > button"})
	if !ok {
		t.Fatal("AddAnnotation should succeed")
	}
	_, ok = store.AddThreadMessage(annotation.ID, "human", "Hello")
	if !ok {
		t.Fatal("AddThreadMessage should succeed")
	}
	store.EmitActionRequested(session.ID, ActionRequest{SessionID: session.ID, Output: "do persisted work"})

	reloaded := NewStore()
	sessions := reloaded.ListSessions()
	if len(sessions) == 0 {
		t.Fatal("expected persisted sessions after reload")
	}
	_, found := reloaded.GetAnnotation(annotation.ID)
	if !found {
		t.Fatal("expected persisted annotation after reload")
	}
	events := reloaded.GetEventsSince(session.ID, 0)
	if len(events) == 0 {
		t.Fatal("expected persisted events after reload")
	}
}

func TestStorePersistenceHelpersWithFailingBackend(t *testing.T) {
	t.Setenv("AGENTATION_STORE", "memory")
	store := NewStore()
	store.persistence = failingBackend{}

	store.persistSessionLocked(Session{ID: "s1", URL: "http://example.com", Status: "active", CreatedAt: nowISO()})
	store.persistAnnotationLocked(Annotation{ID: "a1", SessionID: "s1", Comment: "x", Element: "button", ElementPath: "body > button"})
	store.deleteAnnotationLocked("a1")
	store.persistEventLocked(Event{Sequence: 1, SessionID: "s1", Type: EventAnnotationCreated, Payload: map[string]any{"id": "a1"}})
}

func TestStoreNewStoreFallbackOnPersistenceError(t *testing.T) {
	t.Setenv("AGENTATION_STORE", "sqlite")
	t.Setenv("AGENTATION_DB_PATH", t.TempDir())

	store := NewStore()
	if store.persistence != nil {
		t.Fatal("expected in-memory fallback when SQLite cannot initialize")
	}
}

type failingBackend struct{}

func (f failingBackend) LoadSnapshot() (storeSnapshot, error) {
	return storeSnapshot{}, fmt.Errorf("load error")
}

func (f failingBackend) UpsertSession(session Session) error {
	return fmt.Errorf("session error")
}

func (f failingBackend) UpsertAnnotation(annotation Annotation) error {
	return fmt.Errorf("annotation error")
}

func (f failingBackend) DeleteAnnotation(annotationID string) error {
	return fmt.Errorf("delete error")
}

func (f failingBackend) InsertEvent(event Event) error {
	return fmt.Errorf("event error")
}

func (f failingBackend) Close() error {
	return nil
}
