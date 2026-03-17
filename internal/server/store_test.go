package server

import (
	"testing"
)

func TestStoreCoreFlows(t *testing.T) {
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
	nonBlockingSend(full, Event{Type: EventAnnotationUpdated})
	if len(full) != 1 {
		t.Fatal("nonBlockingSend should not block or add when channel is full")
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
