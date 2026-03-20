package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/api"
)

func TestRunProjectsJSON(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.RequestURI() != "/sessions" {
			t.Fatalf("request URI = %q, want %q", request.URL.RequestURI(), "/sessions")
		}
		_, _ = writer.Write([]byte(`[
			{"id":"s1","projectId":"proj-b"},
			{"id":"s2","projectId":"proj-a"},
			{"id":"s3","projectId":"proj-b"},
			{"id":"s4","projectId":"   "}
		]`))
	}))
	defer testServer.Close()

	client := api.NewClient(testServer.URL)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := RunProjects(context.Background(), client, []string{"--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunProjects returned error: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	mustContain(t, stdout.String(), "[\n  \"proj-a\",\n  \"proj-b\"\n]\n")
}

func TestRunProjectJSON(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.RequestURI() {
		case "/sessions?projectId=proj-1":
			_, _ = writer.Write([]byte(`[
				{"id":"s2","status":"active","url":"https://example.com/2"},
				{"id":"s1","status":"complete","url":"https://example.com/1"}
			]`))
		case "/sessions/s1":
			_, _ = writer.Write([]byte(`{"id":"s1","annotations":[{"id":"a1"}]}`))
		case "/sessions/s2":
			_, _ = writer.Write([]byte(`{"id":"s2","annotations":[{"id":"a2"},{"id":"a3"}]}`))
		default:
			t.Fatalf("unexpected URI: %s", request.URL.RequestURI())
		}
	}))
	defer testServer.Close()

	client := api.NewClient(testServer.URL)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := RunProject(context.Background(), client, []string{"proj-1", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunProject returned error: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	output := stdout.String()
	mustContain(t, output, "\"projectId\": \"proj-1\"")
	mustContain(t, output, "\"sessionCount\": 2")
	mustContain(t, output, "\"annotationCount\": 3")
	mustContain(t, output, "\"id\": \"s1\"")
	mustContain(t, output, "\"id\": \"s2\"")
	if strings.Index(output, "\"id\": \"s1\"") > strings.Index(output, "\"id\": \"s2\"") {
		t.Fatalf("expected sessions to be sorted by id, got output:\n%s", output)
	}
}

func TestRunProjectConcurrentFetchKeepsSortedSessions(t *testing.T) {
	sessionCount := 16
	type session struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		URL    string `json:"url"`
	}

	sessions := make([]session, 0, sessionCount)
	for index := sessionCount; index >= 1; index-- {
		sessions = append(sessions, session{
			ID:     fmt.Sprintf("s%02d", index),
			Status: "active",
			URL:    fmt.Sprintf("https://example.com/%02d", index),
		})
	}

	listPayload, err := json.Marshal(sessions)
	if err != nil {
		t.Fatalf("marshaling sessions payload: %v", err)
	}

	var inFlight int64
	var maxInFlight int64

	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		uri := request.URL.RequestURI()
		switch {
		case uri == "/sessions?projectId=proj-1":
			_, _ = writer.Write(listPayload)
		case strings.HasPrefix(uri, "/sessions/"):
			sessionID := strings.TrimPrefix(uri, "/sessions/")
			current := atomic.AddInt64(&inFlight, 1)
			for {
				previousMax := atomic.LoadInt64(&maxInFlight)
				if current <= previousMax {
					break
				}
				if atomic.CompareAndSwapInt64(&maxInFlight, previousMax, current) {
					break
				}
			}

			time.Sleep(35 * time.Millisecond)
			atomic.AddInt64(&inFlight, -1)

			_, _ = writer.Write([]byte(fmt.Sprintf(`{"id":"%s","annotations":[{"id":"a-%s"}]}`,
				sessionID,
				sessionID,
			)))
		default:
			t.Fatalf("unexpected URI: %s", uri)
		}
	}))
	defer testServer.Close()

	client := api.NewClient(testServer.URL)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err = RunProject(context.Background(), client, []string{"proj-1", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunProject returned error: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	if atomic.LoadInt64(&maxInFlight) <= 1 {
		t.Fatalf("expected concurrent session fetches, max in-flight requests = %d", maxInFlight)
	}
	if atomic.LoadInt64(&maxInFlight) > int64(projectSessionFetchConcurrency) {
		t.Fatalf("max in-flight requests = %d, expected <= %d", maxInFlight, projectSessionFetchConcurrency)
	}

	var summary projectSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("unmarshaling RunProject output: %v", err)
	}

	if summary.SessionCount != sessionCount {
		t.Fatalf("sessionCount = %d, want %d", summary.SessionCount, sessionCount)
	}
	if summary.AnnotationCount != sessionCount {
		t.Fatalf("annotationCount = %d, want %d", summary.AnnotationCount, sessionCount)
	}
	if len(summary.Sessions) != sessionCount {
		t.Fatalf("len(summary.Sessions) = %d, want %d", len(summary.Sessions), sessionCount)
	}
	for index := 1; index < len(summary.Sessions); index++ {
		if summary.Sessions[index-1].ID > summary.Sessions[index].ID {
			t.Fatalf("sessions are not sorted by id: %q before %q", summary.Sessions[index-1].ID, summary.Sessions[index].ID)
		}
	}
}

func TestRunProjectConcurrentFetchReturnsError(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.RequestURI() {
		case "/sessions?projectId=proj-1":
			_, _ = writer.Write([]byte(`[
				{"id":"s1","status":"active","url":"https://example.com/1"},
				{"id":"s2","status":"active","url":"https://example.com/2"},
				{"id":"s3","status":"active","url":"https://example.com/3"}
			]`))
		case "/sessions/s1":
			time.Sleep(25 * time.Millisecond)
			_, _ = writer.Write([]byte(`{"id":"s1","annotations":[]}`))
		case "/sessions/s2":
			http.Error(writer, "boom", http.StatusInternalServerError)
		case "/sessions/s3":
			time.Sleep(25 * time.Millisecond)
			_, _ = writer.Write([]byte(`{"id":"s3","annotations":[]}`))
		default:
			t.Fatalf("unexpected URI: %s", request.URL.RequestURI())
		}
	}))
	defer testServer.Close()

	client := api.NewClient(testServer.URL)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := RunProject(context.Background(), client, []string{"proj-1", "--json"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected RunProject to return an error")
	}
	mustContain(t, err.Error(), `getting session "s2"`)
}

func TestRunPendingText(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.RequestURI() != "/pending?projectId=proj-1" {
			t.Fatalf("request URI = %q, want %q", request.URL.RequestURI(), "/pending?projectId=proj-1")
		}
		_, _ = writer.Write([]byte(`{
			"count":1,
			"annotations":[
				{"id":"ann_1","comment":"Fix spacing","element":"button","elementPath":"body > button"}
			]
		}`))
	}))
	defer testServer.Close()

	client := api.NewClient(testServer.URL)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := RunPending(context.Background(), client, []string{"proj-1"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunPending returned error: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	mustContain(t, stdout.String(), "Pending annotations: 1")
	mustContain(t, stdout.String(), "[1] ann_1")
	mustContain(t, stdout.String(), "Fix spacing")
	mustContain(t, stdout.String(), "Element: button")
}

func TestRunWatchText(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.RequestURI() != "/pending?projectId=proj-1" {
			t.Fatalf("request URI = %q, want %q", request.URL.RequestURI(), "/pending?projectId=proj-1")
		}
		_, _ = writer.Write([]byte(`{
			"count":1,
			"annotations":[
				{"id":"ann_1","sessionId":"s1","comment":"Need update","elementPath":"body > div"}
			]
		}`))
	}))
	defer testServer.Close()

	client := api.NewClient(testServer.URL)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := RunWatch(context.Background(), client, []string{"proj-1"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunWatch returned error: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	mustContain(t, stdout.String(), "Received 1 annotation(s)")
	mustContain(t, stdout.String(), "[1] ann_1")
	mustContain(t, stdout.String(), "Need update")
	mustContain(t, stdout.String(), "Session: s1")
}

func TestRunActionCommands(t *testing.T) {
	var requests []string
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests = append(requests, request.Method+" "+request.URL.Path+" "+strings.TrimSpace(string(body)))

		switch request.URL.Path {
		case "/annotations/ann_1":
			if request.Method != http.MethodPatch {
				t.Fatalf("method = %s, want PATCH", request.Method)
			}
			_, _ = writer.Write([]byte(`{"ok":true}`))
		case "/annotations/ann_1/thread":
			if request.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", request.Method)
			}
			_, _ = writer.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer testServer.Close()

	client := api.NewClient(testServer.URL)
	ctx := context.Background()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := RunAcknowledge(ctx, client, []string{"ann_1", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("RunAcknowledge returned error: %v", err)
	}
	mustContain(t, stdout.String(), `"acknowledged": true`)

	stdout.Reset()
	if err := RunResolve(ctx, client, []string{"ann_1", "--summary", "updated spacing", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("RunResolve returned error: %v", err)
	}
	mustContain(t, stdout.String(), `"resolved": true`)
	mustContain(t, stdout.String(), `"summary": "updated spacing"`)

	stdout.Reset()
	if err := RunDismiss(ctx, client, []string{"ann_1", "--reason", "won't fix", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("RunDismiss returned error: %v", err)
	}
	mustContain(t, stdout.String(), `"dismissed": true`)
	mustContain(t, stdout.String(), `"reason": "won't fix"`)

	stdout.Reset()
	if err := RunReply(ctx, client, []string{"ann_1", "--message", "on it", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("RunReply returned error: %v", err)
	}
	mustContain(t, stdout.String(), `"replied": true`)
	mustContain(t, stdout.String(), `"message": "on it"`)

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	joined := strings.Join(requests, "\n")
	mustContain(t, joined, `PATCH /annotations/ann_1 {"status":"acknowledged"}`)
	mustContain(t, joined, `PATCH /annotations/ann_1 {"resolvedBy":"agent","status":"resolved"}`)
	mustContain(t, joined, `POST /annotations/ann_1/thread {"content":"Resolved: updated spacing","role":"agent"}`)
	mustContain(t, joined, `PATCH /annotations/ann_1 {"resolvedBy":"agent","status":"dismissed"}`)
	mustContain(t, joined, `POST /annotations/ann_1/thread {"content":"Dismissed: won't fix","role":"agent"}`)
	mustContain(t, joined, `POST /annotations/ann_1/thread {"content":"on it","role":"agent"}`)
}

func mustContain(t *testing.T, text, fragment string) {
	t.Helper()
	if !strings.Contains(text, fragment) {
		t.Fatalf("expected text to contain %q\n--- text ---\n%s", fragment, text)
	}
}
