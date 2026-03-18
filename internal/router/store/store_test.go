package store

import (
	"errors"
	"testing"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/router/model"
)

func TestResolveByProjectID(t *testing.T) {
	now := time.Date(2026, time.March, 17, 12, 0, 0, 0, time.UTC)
	registry := NewRegistry(time.Minute)
	registry.now = func() time.Time {
		return now
	}

	registry.Register(model.RegisterRequest{
		SessionID: "session-a",
		ProjectID: "project-a",
		Root:      "/repo/a",
		Endpoint:  "http://127.0.0.1:9011",
	})
	registry.Register(model.RegisterRequest{
		SessionID: "session-b",
		ProjectID: "project-b",
		Root:      "/repo/b",
		Endpoint:  "http://127.0.0.1:9012",
	})

	resolved, error := registry.Resolve("project-b", "", "")
	if error != nil {
		t.Fatalf("Resolve returned error: %v", error)
	}
	if resolved.SessionID != "session-b" {
		t.Fatalf("Resolve returned session %q, want %q", resolved.SessionID, "session-b")
	}
}

func TestResolveByOriginStickyMapping(t *testing.T) {
	registry := NewRegistry(time.Minute)
	registry.Register(model.RegisterRequest{
		SessionID: "session-a",
		ProjectID: "project-a",
		Root:      "/repo/a",
		Endpoint:  "http://127.0.0.1:9011",
	})

	_, error := registry.Resolve("", "/repo/a/src/Button.tsx", "https://app-a.local")
	if error != nil {
		t.Fatalf("Resolve returned error: %v", error)
	}

	resolved, error := registry.Resolve("", "", "https://app-a.local")
	if error != nil {
		t.Fatalf("Resolve returned error: %v", error)
	}
	if resolved.SessionID != "session-a" {
		t.Fatalf("Resolve returned session %q, want %q", resolved.SessionID, "session-a")
	}
}

func TestResolveByBestRootMatch(t *testing.T) {
	registry := NewRegistry(time.Minute)
	registry.Register(model.RegisterRequest{
		SessionID: "session-a",
		ProjectID: "project-a",
		Root:      "/repo",
		Endpoint:  "http://127.0.0.1:9011",
	})
	registry.Register(model.RegisterRequest{
		SessionID: "session-b",
		ProjectID: "project-b",
		Root:      "/repo/app",
		Endpoint:  "http://127.0.0.1:9012",
	})

	resolved, error := registry.Resolve("", "/repo/app/src/Button.tsx", "")
	if error != nil {
		t.Fatalf("Resolve returned error: %v", error)
	}
	if resolved.SessionID != "session-b" {
		t.Fatalf("Resolve returned session %q, want %q", resolved.SessionID, "session-b")
	}
}

func TestResolveAmbiguousWithoutRoutingHints(t *testing.T) {
	registry := NewRegistry(time.Minute)
	registry.Register(model.RegisterRequest{
		SessionID: "session-a",
		ProjectID: "project-a",
		Root:      "/repo/a",
		Endpoint:  "http://127.0.0.1:9011",
	})
	registry.Register(model.RegisterRequest{
		SessionID: "session-b",
		ProjectID: "project-b",
		Root:      "/repo/b",
		Endpoint:  "http://127.0.0.1:9012",
	})

	_, error := registry.Resolve("", "src/Button.tsx", "")
	if !errors.Is(error, ErrAmbiguousSession) {
		t.Fatalf("Resolve returned error %v, want %v", error, ErrAmbiguousSession)
	}
}

func TestPrunesStaleSessions(t *testing.T) {
	currentTime := time.Date(2026, time.March, 17, 12, 0, 0, 0, time.UTC)
	registry := NewRegistry(2 * time.Second)
	registry.now = func() time.Time {
		return currentTime
	}

	registry.Register(model.RegisterRequest{
		SessionID: "session-a",
		ProjectID: "project-a",
		Root:      "/repo/a",
		Endpoint:  "http://127.0.0.1:9011",
	})

	currentTime = currentTime.Add(3 * time.Second)

	sessions := registry.List()
	if len(sessions) != 0 {
		t.Fatalf("List returned %d sessions, want 0", len(sessions))
	}

	_, error := registry.Resolve("project-a", "", "")
	if !errors.Is(error, ErrNoMatchingSession) {
		t.Fatalf("Resolve returned error %v, want %v", error, ErrNoMatchingSession)
	}
}

func TestRegisterWithOutcome(t *testing.T) {
	registry := NewRegistry(time.Minute)

	payload := model.RegisterRequest{
		SessionID: "session-a",
		ProjectID: "project-a",
		Root:      "/repo/a",
		Endpoint:  "http://127.0.0.1:9011",
	}

	_, firstOutcome := registry.RegisterWithOutcome(payload)
	if firstOutcome != RegisterOutcomeNew {
		t.Fatalf("first register outcome %q, want %q", firstOutcome, RegisterOutcomeNew)
	}

	_, secondOutcome := registry.RegisterWithOutcome(payload)
	if secondOutcome != RegisterOutcomeHeartbeat {
		t.Fatalf("second register outcome %q, want %q", secondOutcome, RegisterOutcomeHeartbeat)
	}

	payload.Endpoint = "http://127.0.0.1:9012"
	_, thirdOutcome := registry.RegisterWithOutcome(payload)
	if thirdOutcome != RegisterOutcomeUpdated {
		t.Fatalf("third register outcome %q, want %q", thirdOutcome, RegisterOutcomeUpdated)
	}
}
