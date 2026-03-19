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
			_, _ = writer.Write([]byte(`[{"id":"s1","url":"http://example.com","status":"active","createdAt":"now"}]`))
		case "/sessions/s1":
			if request.Method != http.MethodGet {
				t.Fatalf("method = %s, want GET", request.Method)
			}
			_, _ = writer.Write([]byte(`{"id":"s1","url":"http://example.com","status":"active","createdAt":"now","annotations":[]}`))
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
