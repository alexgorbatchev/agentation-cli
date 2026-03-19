package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/api"
)

func TestRunProjectFetchesSessionDetailsConcurrentlyWithWorkerLimit(t *testing.T) {
	t.Parallel()

	sessions := make([]api.Session, 0, 12)
	annotationCounts := make(map[string]int, 12)
	totalAnnotations := 0
	for i := range 12 {
		id := fmt.Sprintf("session-%02d", i)
		annotationCount := (i % 4) + 1
		totalAnnotations += annotationCount
		sessions = append(sessions, api.Session{
			ID:     id,
			Status: "active",
			URL:    "https://example.com/" + id,
		})
		annotationCounts[id] = annotationCount
	}

	var activeRequests int64
	var maxConcurrent int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Helper()
		switch {
		case r.URL.Path == "/sessions":
			if got := r.URL.Query().Get("projectId"); got != "project-1" {
				t.Fatalf("projectId query = %q, want %q", got, "project-1")
			}
			writeProjectTestJSON(t, w, sessions)
		case strings.HasPrefix(r.URL.Path, "/sessions/"):
			sessionID := strings.TrimPrefix(r.URL.Path, "/sessions/")
			annotationCount, ok := annotationCounts[sessionID]
			if !ok {
				t.Fatalf("unexpected session id %q", sessionID)
			}

			current := atomic.AddInt64(&activeRequests, 1)
			defer atomic.AddInt64(&activeRequests, -1)
			for {
				max := atomic.LoadInt64(&maxConcurrent)
				if current <= max || atomic.CompareAndSwapInt64(&maxConcurrent, max, current) {
					break
				}
			}

			time.Sleep(30 * time.Millisecond)

			annotations := make([]api.Annotation, annotationCount)
			writeProjectTestJSON(t, w, api.SessionWithAnnotations{
				Session: api.Session{
					ID:     sessionID,
					Status: "active",
					URL:    "https://example.com/" + sessionID,
				},
				Annotations: annotations,
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runProject(t.Context(), api.NewClient(server.URL), []string{"project-1", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runProject() error = %v", err)
	}

	var summary projectSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("unmarshal project summary: %v", err)
	}

	if summary.ProjectID != "project-1" {
		t.Fatalf("summary.ProjectID = %q, want %q", summary.ProjectID, "project-1")
	}
	if summary.SessionCount != len(sessions) {
		t.Fatalf("summary.SessionCount = %d, want %d", summary.SessionCount, len(sessions))
	}
	if summary.AnnotationCount != totalAnnotations {
		t.Fatalf("summary.AnnotationCount = %d, want %d", summary.AnnotationCount, totalAnnotations)
	}

	gotSessionIDs := make([]string, 0, len(summary.Sessions))
	for _, session := range summary.Sessions {
		gotSessionIDs = append(gotSessionIDs, session.ID)
	}

	wantSessionIDs := slices.Clone(gotSessionIDs)
	slices.Sort(wantSessionIDs)
	if !slices.Equal(gotSessionIDs, wantSessionIDs) {
		t.Fatalf("summary sessions are not sorted by id: got %v", gotSessionIDs)
	}

	if maxConcurrent <= 1 {
		t.Fatalf("max concurrent session fetches = %d, want > 1", maxConcurrent)
	}
	if maxConcurrent > int64(projectSessionFetchWorkerLimit) {
		t.Fatalf("max concurrent session fetches = %d, want <= %d", maxConcurrent, projectSessionFetchWorkerLimit)
	}
}

func TestRunProjectReturnsFirstSessionErrorByInputOrder(t *testing.T) {
	t.Parallel()

	sessions := []api.Session{
		{ID: "session-a", Status: "active", URL: "https://example.com/session-a"},
		{ID: "session-b", Status: "active", URL: "https://example.com/session-b"},
		{ID: "session-c", Status: "active", URL: "https://example.com/session-c"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Helper()
		switch r.URL.Path {
		case "/sessions":
			writeProjectTestJSON(t, w, sessions)
		case "/sessions/session-a":
			time.Sleep(60 * time.Millisecond)
			http.Error(w, "session-a failed", http.StatusInternalServerError)
		case "/sessions/session-b":
			time.Sleep(5 * time.Millisecond)
			http.Error(w, "session-b failed", http.StatusInternalServerError)
		case "/sessions/session-c":
			writeProjectTestJSON(t, w, api.SessionWithAnnotations{
				Session: api.Session{ID: "session-c", Status: "active", URL: "https://example.com/session-c"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runProject(t.Context(), api.NewClient(server.URL), []string{"project-1", "--json"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("runProject() error = nil, want non-nil")
	}

	if !strings.Contains(err.Error(), "getting session \"session-a\"") {
		t.Fatalf("runProject() error = %q, want to mention session-a", err)
	}
}

func writeProjectTestJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()

	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Fatalf("encoding response: %v", err)
	}
}
