package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClientUsesDefaultBaseURL(t *testing.T) {
	client := NewClient("   ")
	if client.baseURL != "http://localhost:4747" {
		t.Fatalf("client.baseURL = %q, want %q", client.baseURL, "http://localhost:4747")
	}
}

func TestListSessionsAndGetSession(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/sessions":
			if request.Method != http.MethodGet {
				t.Fatalf("method = %s, want GET", request.Method)
			}
			_, _ = writer.Write([]byte(`[{"id":"s1","url":"http://example.com","status":"active","createdAt":1774569600000}]`))
		case "/sessions/s1":
			if request.Method != http.MethodGet {
				t.Fatalf("method = %s, want GET", request.Method)
			}
			_, _ = writer.Write([]byte(`{"id":"s1","url":"http://example.com","status":"active","createdAt":1774569600000,"annotations":[]}`))
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer testServer.Close()

	client := NewClient(testServer.URL)

	sessions, err := client.ListSessions(context.Background(), "")
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "s1" {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}

	session, err := client.GetSession(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if session.ID != "s1" {
		t.Fatalf("session.ID = %q, want %q", session.ID, "s1")
	}
}

func TestListSessionsByProjectID(t *testing.T) {
	var requestURI string
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestURI = request.URL.RequestURI()
		_, _ = writer.Write([]byte(`[]`))
	}))
	defer testServer.Close()

	client := NewClient(testServer.URL)
	_, err := client.ListSessions(context.Background(), "project-1")
	if err != nil {
		t.Fatalf("ListSessions(project) returned error: %v", err)
	}
	if requestURI != "/sessions?projectId=project-1" {
		t.Fatalf("requestURI = %q, want %q", requestURI, "/sessions?projectId=project-1")
	}
}

func TestGetPendingAllAndBySession(t *testing.T) {
	calls := make(map[string]int)
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestPath := request.URL.RequestURI()
		calls[requestPath]++
		switch requestPath {
		case "/pending":
			_, _ = writer.Write([]byte(`{"count":1,"annotations":[{"id":"a1","sessionId":"s1","comment":"Fix","element":"button","elementPath":"body > button"}]}`))
		case "/pending?projectId=p1":
			_, _ = writer.Write([]byte(`{"count":0,"annotations":[]}`))
		case "/sessions/s1/pending":
			_, _ = writer.Write([]byte(`{"count":0,"annotations":[]}`))
		default:
			t.Fatalf("unexpected path: %s", requestPath)
		}
	}))
	defer testServer.Close()

	client := NewClient(testServer.URL)

	pendingAll, err := client.GetPending(context.Background(), "", "")
	if err != nil {
		t.Fatalf("GetPending(all) returned error: %v", err)
	}
	if pendingAll.Count != 1 {
		t.Fatalf("pendingAll.Count = %d, want 1", pendingAll.Count)
	}

	pendingProject, err := client.GetPending(context.Background(), "", "p1")
	if err != nil {
		t.Fatalf("GetPending(project) returned error: %v", err)
	}
	if pendingProject.Count != 0 {
		t.Fatalf("pendingProject.Count = %d, want 0", pendingProject.Count)
	}

	pendingSession, err := client.GetPending(context.Background(), "s1", "")
	if err != nil {
		t.Fatalf("GetPending(session) returned error: %v", err)
	}
	if pendingSession.Count != 0 {
		t.Fatalf("pendingSession.Count = %d, want 0", pendingSession.Count)
	}

	if calls["/pending"] != 1 || calls["/pending?projectId=p1"] != 1 || calls["/sessions/s1/pending"] != 1 {
		t.Fatalf("unexpected calls: %#v", calls)
	}
}

func TestClientPreservesRichJSONFields(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.RequestURI() {
		case "/sessions/s1":
			_, _ = writer.Write([]byte(`{
				"id":"s1",
				"url":"http://example.com",
				"status":"active",
				"createdAt":1774483200000,
				"metadata":{"branch":"main"},
				"annotations":[{
					"id":"a1",
					"sessionId":"s1",
					"x":12.5,
					"y":240,
					"comment":"Fix button",
					"element":"button",
					"elementPath":"body > button",
					"timestamp":1774490572310,
					"boundingBox":{"x":1,"y":2,"width":3,"height":4},
					"cssClasses":"btn primary",
					"nearbyElements":"div.toolbar",
					"computedStyles":"color: blue",
					"createdAt":1774483201000,
					"updatedAt":1774483202000,
					"resolvedAt":1774483203000,
					"resolvedBy":"agent",
					"authorId":"u1",
					"thread":[{"id":"m1","role":"human","content":"please fix","timestamp":1774490572311}]
				}]
			}`))
		case "/pending?projectId=p-rich":
			_, _ = writer.Write([]byte(`{
				"count":1,
				"annotations":[{
					"id":"a2",
					"sessionId":"s2",
					"comment":"Need polish",
					"element":"div",
					"elementPath":"body > div",
					"timestamp":1774490572999,
					"reactComponents":"App > Panel",
					"sourceFile":"src/Panel.tsx:10",
					"elementBoundingBoxes":[{"x":10,"y":20,"width":30,"height":40}]
				}]
			}`))
		default:
			t.Fatalf("unexpected path: %s", request.URL.RequestURI())
		}
	}))
	defer testServer.Close()

	client := NewClient(testServer.URL)

	session, err := client.GetSession(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if session.Metadata["branch"] != "main" {
		t.Fatalf("session.Metadata[branch] = %#v, want %q", session.Metadata["branch"], "main")
	}
	if len(session.Annotations) != 1 {
		t.Fatalf("annotation count = %d, want 1", len(session.Annotations))
	}
	sessionAnnotation := session.Annotations[0]
	if sessionAnnotation.X != 12.5 || sessionAnnotation.Y != 240 {
		t.Fatalf("annotation coordinates = (%v, %v), want (12.5, 240)", sessionAnnotation.X, sessionAnnotation.Y)
	}
	if sessionAnnotation.BoundingBox["width"] != float64(3) {
		t.Fatalf("bounding box width = %#v, want 3", sessionAnnotation.BoundingBox["width"])
	}
	if sessionAnnotation.ComputedStyles != "color: blue" {
		t.Fatalf("computed styles = %q, want %q", sessionAnnotation.ComputedStyles, "color: blue")
	}
	if sessionAnnotation.UpdatedAt != 1774483202000 || sessionAnnotation.ResolvedAt != 1774483203000 {
		t.Fatalf("unexpected lifecycle timestamps: updatedAt=%d resolvedAt=%d", sessionAnnotation.UpdatedAt, sessionAnnotation.ResolvedAt)
	}
	if len(sessionAnnotation.Thread) != 1 || sessionAnnotation.Thread[0].Timestamp != 1774490572311 {
		t.Fatalf("thread = %#v, want preserved numeric timestamp", sessionAnnotation.Thread)
	}

	pending, err := client.GetPending(context.Background(), "", "p-rich")
	if err != nil {
		t.Fatalf("GetPending returned error: %v", err)
	}
	if pending.Count != 1 {
		t.Fatalf("pending.Count = %d, want 1", pending.Count)
	}
	pendingAnnotation := pending.Annotations[0]
	if pendingAnnotation.ReactComponents != "App > Panel" {
		t.Fatalf("reactComponents = %q, want %q", pendingAnnotation.ReactComponents, "App > Panel")
	}
	if pendingAnnotation.SourceFile != "src/Panel.tsx:10" {
		t.Fatalf("sourceFile = %q, want %q", pendingAnnotation.SourceFile, "src/Panel.tsx:10")
	}
	if len(pendingAnnotation.ElementBoundingBoxes) != 1 {
		t.Fatalf("elementBoundingBoxes length = %d, want 1", len(pendingAnnotation.ElementBoundingBoxes))
	}
}

func TestAcknowledgeResolveDismissAndReply(t *testing.T) {
	var requests []string
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests = append(requests, request.Method+" "+request.URL.Path+" "+strings.TrimSpace(string(body)))

		if request.URL.Path == "/annotations/a1" && request.Method == http.MethodPatch {
			_, _ = writer.Write([]byte(`{"id":"a1"}`))
			return
		}
		if request.URL.Path == "/annotations/a1/thread" && request.Method == http.MethodPost {
			_, _ = writer.Write([]byte(`{"id":"a1"}`))
			return
		}
		t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
	}))
	defer testServer.Close()

	client := NewClient(testServer.URL)
	ctx := context.Background()

	if err := client.Acknowledge(ctx, "a1"); err != nil {
		t.Fatalf("Acknowledge returned error: %v", err)
	}
	if err := client.Resolve(ctx, "a1", "updated spacing"); err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if err := client.Resolve(ctx, "a1", "   "); err != nil {
		t.Fatalf("Resolve(empty summary) returned error: %v", err)
	}
	if err := client.Dismiss(ctx, "a1", "won't fix"); err != nil {
		t.Fatalf("Dismiss returned error: %v", err)
	}
	if err := client.Reply(ctx, "a1", "on it"); err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}

	joined := strings.Join(requests, "\n")
	mustContain(t, joined, `PATCH /annotations/a1 {"status":"acknowledged"}`)
	mustContain(t, joined, `PATCH /annotations/a1 {"resolvedBy":"agent","status":"resolved"}`)
	mustContain(t, joined, `POST /annotations/a1/thread {"content":"Resolved: updated spacing","role":"agent"}`)
	mustContain(t, joined, `PATCH /annotations/a1 {"resolvedBy":"agent","status":"dismissed"}`)
	mustContain(t, joined, `POST /annotations/a1/thread {"content":"Dismissed: won't fix","role":"agent"}`)
	mustContain(t, joined, `POST /annotations/a1/thread {"content":"on it","role":"agent"}`)
}

func TestDoJSONErrorPaths(t *testing.T) {
	badStatus := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte("upstream error"))
	}))
	defer badStatus.Close()

	client := NewClient(badStatus.URL)
	err := client.Acknowledge(context.Background(), "a1")
	if err == nil || !strings.Contains(err.Error(), "http 502") {
		t.Fatalf("expected http 502 error, got %v", err)
	}

	invalidJSON := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("not-json"))
	}))
	defer invalidJSON.Close()

	client = NewClient(invalidJSON.URL)
	_, err = client.ListSessions(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "decoding response") {
		t.Fatalf("expected decode error, got %v", err)
	}

	noBodyError := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer noBodyError.Close()

	client = NewClient(noBodyError.URL)
	_, err = client.GetSession(context.Background(), "s1")
	if err == nil || !strings.Contains(err.Error(), "Forbidden") {
		t.Fatalf("expected status text fallback, got %v", err)
	}

	client = NewClient("http://127.0.0.1:1")
	err = client.Acknowledge(context.Background(), "a1")
	if err == nil || !strings.Contains(err.Error(), "sending request") {
		t.Fatalf("expected transport error, got %v", err)
	}
}

func TestClientActionAndLookupErrorPaths(t *testing.T) {
	failingServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/sessions/missing" {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte("missing"))
			return
		}
		if request.URL.Path == "/annotations/a1" && request.Method == http.MethodPatch {
			_, _ = writer.Write([]byte(`{"id":"a1"}`))
			return
		}
		if request.URL.Path == "/annotations/a1/thread" && request.Method == http.MethodPost {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte("thread failed"))
			return
		}
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte("unexpected"))
	}))
	defer failingServer.Close()

	client := NewClient(failingServer.URL)

	_, err := client.GetSession(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "getting session") {
		t.Fatalf("expected get session error, got %v", err)
	}

	err = client.Resolve(context.Background(), "a1", "summary")
	if err == nil || !strings.Contains(err.Error(), "adding resolution summary") {
		t.Fatalf("expected resolve thread error, got %v", err)
	}

	err = client.Dismiss(context.Background(), "a1", "reason")
	if err == nil || !strings.Contains(err.Error(), "adding dismissal message") {
		t.Fatalf("expected dismiss thread error, got %v", err)
	}

	err = client.Reply(context.Background(), "a1", "hello")
	if err == nil || !strings.Contains(err.Error(), "replying to annotation") {
		t.Fatalf("expected reply error, got %v", err)
	}
}

func TestMarshalBody(t *testing.T) {
	reader, err := marshalBody(nil)
	if err != nil {
		t.Fatalf("marshalBody(nil) error: %v", err)
	}
	if reader != nil {
		t.Fatal("marshalBody(nil) should return nil reader")
	}

	reader, err = marshalBody(map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("marshalBody(map) error: %v", err)
	}
	payload, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}

	parsed := make(map[string]any)
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if parsed["a"].(float64) != 1 {
		t.Fatalf("parsed[a] = %v, want 1", parsed["a"])
	}
}

func mustContain(t *testing.T, text, fragment string) {
	t.Helper()
	if !strings.Contains(text, fragment) {
		t.Fatalf("expected %q to contain %q", text, fragment)
	}
}
