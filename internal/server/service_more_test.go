package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHTTPAPIBranches(t *testing.T) {
	service := NewService("127.0.0.1:0", slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(service.httpServer.Handler)
	defer ts.Close()

	call := func(method, path string, body any) (*http.Response, string) {
		t.Helper()
		var reader io.Reader
		if body != nil {
			payload, _ := json.Marshal(body)
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequest(method, ts.URL+path, reader)
		if err != nil {
			t.Fatalf("NewRequest failed: %v", err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		content, _ := io.ReadAll(resp.Body)
		return resp, string(content)
	}

	resp, _ := call(http.MethodGet, "/health", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/health status = %d", resp.StatusCode)
	}

	resp, _ = call(http.MethodGet, "/status", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/status status = %d", resp.StatusCode)
	}

	resp, _ = call(http.MethodOptions, "/sessions", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("OPTIONS /sessions status = %d", resp.StatusCode)
	}

	resp, body := call(http.MethodPost, "/sessions", map[string]any{})
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "url is required") {
		t.Fatalf("expected create session validation error, got %d %s", resp.StatusCode, body)
	}

	resp, body = call(http.MethodGet, "/sessions/missing", nil)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(body, "Session not found") {
		t.Fatalf("expected missing session, got %d %s", resp.StatusCode, body)
	}

	resp, body = call(http.MethodPost, "/sessions", map[string]any{"url": "http://example.com"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create session status = %d body=%s", resp.StatusCode, body)
	}
	var session Session
	if err := json.Unmarshal([]byte(body), &session); err != nil {
		t.Fatalf("unmarshal session failed: %v", err)
	}

	resp, body = call(http.MethodPost, "/sessions/missing/annotations", map[string]any{"comment": "c", "element": "button", "elementPath": "body > button"})
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(body, "Session not found") {
		t.Fatalf("expected missing session annotation add, got %d %s", resp.StatusCode, body)
	}

	resp, body = call(http.MethodPost, "/sessions/"+session.ID+"/annotations", map[string]any{"comment": "", "element": "button", "elementPath": "body > button"})
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "required") {
		t.Fatalf("expected add annotation validation error, got %d %s", resp.StatusCode, body)
	}

	resp, body = call(http.MethodPost, "/sessions/"+session.ID+"/annotations", map[string]any{
		"comment":     "Fix",
		"element":     "button",
		"elementPath": "body > button",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add annotation status = %d body=%s", resp.StatusCode, body)
	}
	var annotation Annotation
	_ = json.Unmarshal([]byte(body), &annotation)

	resp, body = call(http.MethodGet, "/annotations/missing", nil)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(body, "Annotation not found") {
		t.Fatalf("expected missing annotation, got %d %s", resp.StatusCode, body)
	}

	resp, body = call(http.MethodPatch, "/annotations/"+annotation.ID, map[string]any{"status": "acknowledged"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch annotation status = %d body=%s", resp.StatusCode, body)
	}

	resp, body = call(http.MethodPatch, "/annotations/missing", map[string]any{"status": "acknowledged"})
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(body, "Annotation not found") {
		t.Fatalf("expected patch missing annotation, got %d %s", resp.StatusCode, body)
	}

	resp, body = call(http.MethodPost, "/annotations/"+annotation.ID+"/thread", map[string]any{"role": "robot", "content": "x"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid role, got %d %s", resp.StatusCode, body)
	}

	resp, body = call(http.MethodPost, "/annotations/"+annotation.ID+"/thread", map[string]any{"role": "human", "content": "   "})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected empty content, got %d %s", resp.StatusCode, body)
	}

	resp, body = call(http.MethodPost, "/annotations/missing/thread", map[string]any{"role": "human", "content": "x"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected missing thread target, got %d %s", resp.StatusCode, body)
	}

	resp, body = call(http.MethodPost, "/annotations/"+annotation.ID+"/thread", map[string]any{"role": "human", "content": "please fix hover"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("thread create status = %d body=%s", resp.StatusCode, body)
	}

	resp, body = call(http.MethodGet, "/sessions/missing/pending", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected missing pending session, got %d %s", resp.StatusCode, body)
	}

	resp, _ = call(http.MethodGet, "/sessions/"+session.ID+"/pending", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("session pending status = %d", resp.StatusCode)
	}

	resp, _ = call(http.MethodGet, "/pending", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("all pending status = %d", resp.StatusCode)
	}

	resp, body = call(http.MethodPost, "/sessions/missing/action", map[string]any{"output": "run"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected missing action session, got %d %s", resp.StatusCode, body)
	}

	resp, body = call(http.MethodPost, "/sessions/"+session.ID+"/action", map[string]any{"output": ""})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected action output validation, got %d %s", resp.StatusCode, body)
	}

	resp, _ = call(http.MethodPost, "/sessions/"+session.ID+"/action", map[string]any{"output": "run it"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("action request status = %d", resp.StatusCode)
	}

	resp, body = call(http.MethodDelete, "/annotations/missing", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected delete missing annotation, got %d %s", resp.StatusCode, body)
	}

	resp, _ = call(http.MethodDelete, "/annotations/"+annotation.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete annotation status = %d", resp.StatusCode)
	}
}

func TestServiceUtilitiesAndSSEHelpers(t *testing.T) {
	service := NewService("127.0.0.1:0", slog.New(slog.NewTextHandler(io.Discard, nil)))

	if parseLastEventID("") != 0 || parseLastEventID("bad") != 0 || parseLastEventID("-1") != 0 || parseLastEventID("2") != 2 {
		t.Fatal("parseLastEventID should handle empty/invalid/negative/valid values")
	}

	if valueOr("", "fallback") != "fallback" || valueOr("x", "fallback") != "x" {
		t.Fatal("valueOr returned unexpected values")
	}

	if sessionMatchesDomain(Session{URL: "::bad-url::"}, "example.com") {
		t.Fatal("invalid URL should not match domain")
	}
	if !sessionMatchesDomain(Session{URL: "http://example.com/path"}, "example.com") {
		t.Fatal("valid URL should match domain")
	}

	if service.eventMatchesDomain(Event{SessionID: "missing"}, "example.com") {
		t.Fatal("missing session should not match domain")
	}

	s := service.store.CreateSession("http://example.com/page", "")
	if !service.eventMatchesDomain(Event{SessionID: s.ID}, "example.com") {
		t.Fatal("existing session event should match domain")
	}

	nonFlusher := &headerOnlyWriter{header: make(http.Header)}
	if startSSE(nonFlusher) {
		t.Fatal("startSSE should fail when Flusher is not implemented")
	}

	flusher := newBufferSSEWriter(false)
	if !startSSE(flusher) {
		t.Fatal("startSSE should succeed for flusher writer")
	}

	if err := writeSSEEvent(flusher, Event{Type: EventAnnotationCreated, Sequence: 1, Payload: map[string]any{"id": "a1"}}); err != nil {
		t.Fatalf("writeSSEEvent failed: %v", err)
	}
	if err := writeSSECustomEvent(flusher, "sync.complete", map[string]any{"count": 1}); err != nil {
		t.Fatalf("writeSSECustomEvent failed: %v", err)
	}
	if err := writeSSEComment(flusher, "ping"); err != nil {
		t.Fatalf("writeSSEComment failed: %v", err)
	}

	failingWriter := newBufferSSEWriter(true)
	if err := writeSSEEvent(failingWriter, Event{Type: EventAnnotationCreated}); err == nil {
		t.Fatal("writeSSEEvent should fail when writer errors")
	}
	if err := writeSSECustomEvent(failingWriter, "x", map[string]any{}); err == nil {
		t.Fatal("writeSSECustomEvent should fail when writer errors")
	}
	if err := writeSSEComment(failingWriter, "x"); err == nil {
		t.Fatal("writeSSEComment should fail when writer errors")
	}

	badRequest := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
	var payload map[string]any
	if err := decodeBody(badRequest, &payload); err == nil {
		t.Fatal("decodeBody should fail for invalid JSON")
	}
	goodRequest := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":1}`))
	if err := decodeBody(goodRequest, &payload); err != nil {
		t.Fatalf("decodeBody should parse valid JSON: %v", err)
	}

	jsonWriter := httptest.NewRecorder()
	writeError(jsonWriter, http.StatusBadRequest, "oops")
	if jsonWriter.Code != http.StatusBadRequest || !strings.Contains(jsonWriter.Body.String(), "oops") {
		t.Fatalf("writeError response unexpected: status=%d body=%s", jsonWriter.Code, jsonWriter.Body.String())
	}
}

func TestStreamFunctionsAndSync(t *testing.T) {
	service := NewService("127.0.0.1:0", slog.New(slog.NewTextHandler(io.Discard, nil)))
	s1 := service.store.CreateSession("http://example.com/a", "")
	s2 := service.store.CreateSession("http://other.com/b", "")

	_, _ = service.store.AddAnnotation(s1.ID, Annotation{Comment: "A", Element: "button", ElementPath: "body > button"})
	_, _ = service.store.AddAnnotation(s2.ID, Annotation{Comment: "B", Element: "div", ElementPath: "body > div"})

	writer := newBufferSSEWriter(false)
	service.sendInitialSync(writer, "example.com")
	if !strings.Contains(writer.String(), "sync.complete") || !strings.Contains(writer.String(), "annotation.created") {
		t.Fatalf("sendInitialSync output missing expected events: %s", writer.String())
	}

	events := make(chan Event, 2)
	events <- Event{Type: EventAnnotationCreated, SessionID: s1.ID, Sequence: 2, Payload: map[string]any{"id": "a1"}}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	service.streamEvents(ctx, writer, events)

	globalWriter := newBufferSSEWriter(false)
	globalEvents := make(chan Event, 3)
	globalEvents <- Event{Type: EventAnnotationCreated, SessionID: s2.ID, Sequence: 3, Payload: map[string]any{"id": "skip"}}
	globalEvents <- Event{Type: EventAnnotationCreated, SessionID: s1.ID, Sequence: 4, Payload: map[string]any{"id": "keep"}}
	ctx2, cancel2 := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel2()
	}()
	service.streamGlobalEvents(ctx2, globalWriter, globalEvents, "example.com")
	if strings.Contains(globalWriter.String(), "skip") {
		t.Fatal("streamGlobalEvents should filter unmatched domains")
	}
	if !strings.Contains(globalWriter.String(), "keep") {
		t.Fatal("streamGlobalEvents should include matching domain event")
	}

	errorWriter := newBufferSSEWriter(true)
	errorEvents := make(chan Event, 1)
	errorEvents <- Event{Type: EventAnnotationCreated, SessionID: s1.ID, Sequence: 5, Payload: map[string]any{"id": "x"}}
	service.streamEvents(context.Background(), errorWriter, errorEvents)
}

func TestSessionAndGlobalEventHandlersDirect(t *testing.T) {
	service := NewService("127.0.0.1:0", slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := service.store.CreateSession("http://example.com/page", "")
	_, _ = service.store.AddAnnotation(session.ID, Annotation{Comment: "A", Element: "button", ElementPath: "body > button"})
	_, _ = service.store.AddAnnotation(session.ID, Annotation{Comment: "B", Element: "div", ElementPath: "body > div"})

	nonFlusher := &headerOnlyWriter{header: make(http.Header)}
	req := httptest.NewRequest(http.MethodGet, "/sessions/"+session.ID+"/events", nil)
	req.SetPathValue("id", session.ID)
	service.handleSessionEvents(nonFlusher, req)
	if nonFlusher.status != http.StatusInternalServerError {
		t.Fatalf("expected 500 for non-flusher writer, got %d", nonFlusher.status)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/sessions/missing/events", nil)
	missingReq.SetPathValue("id", "missing")
	missingWriter := httptest.NewRecorder()
	service.handleSessionEvents(missingWriter, missingReq)
	if missingWriter.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing session events, got %d", missingWriter.Code)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancelWriter := newBufferSSEWriter(false)
	replayReq := httptest.NewRequest(http.MethodGet, "/sessions/"+session.ID+"/events?agent=true", nil).WithContext(ctx)
	replayReq.Header.Set("Last-Event-ID", "1")
	replayReq.SetPathValue("id", session.ID)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		service.handleSessionEvents(cancelWriter, replayReq)
	}()
	time.Sleep(60 * time.Millisecond)
	cancel()
	wg.Wait()

	if service.activeListeners != 0 || service.agentListeners != 0 {
		t.Fatal("listener counters should return to zero after disconnect")
	}
	if !strings.Contains(cancelWriter.String(), "annotation.created") {
		t.Fatalf("expected replay/output events, got: %s", cancelWriter.String())
	}

	globalNonFlusher := &headerOnlyWriter{header: make(http.Header)}
	globalReq := httptest.NewRequest(http.MethodGet, "/events", nil)
	service.handleGlobalEvents(globalNonFlusher, globalReq)
	if globalNonFlusher.status != http.StatusInternalServerError {
		t.Fatalf("expected 500 for global non-flusher, got %d", globalNonFlusher.status)
	}

	globalCtx, globalCancel := context.WithCancel(context.Background())
	globalWriter := newBufferSSEWriter(false)
	globalRequest := httptest.NewRequest(http.MethodGet, "/events?agent=true&domain=example.com", nil).WithContext(globalCtx)
	wg.Add(1)
	go func() {
		defer wg.Done()
		service.handleGlobalEvents(globalWriter, globalRequest)
	}()
	time.Sleep(60 * time.Millisecond)
	globalCancel()
	wg.Wait()

	if !strings.Contains(globalWriter.String(), "sync.complete") {
		t.Fatalf("expected sync.complete in global stream output: %s", globalWriter.String())
	}
}

func TestListenAndShutdown(t *testing.T) {
	service := NewService("127.0.0.1:0", slog.New(slog.NewTextHandler(io.Discard, nil)))

	errCh := make(chan error, 1)
	go func() {
		errCh <- service.ListenAndServe()
	}()

	time.Sleep(80 * time.Millisecond)
	shutdownErr := service.Shutdown(context.Background())
	if shutdownErr != nil {
		t.Fatalf("Shutdown returned error: %v", shutdownErr)
	}

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("ListenAndServe returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ListenAndServe to return")
	}
}

type headerOnlyWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *headerOnlyWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *headerOnlyWriter) WriteHeader(status int) {
	w.status = status
}

func (w *headerOnlyWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

type bufferSSEWriter struct {
	header    http.Header
	status    int
	body      bytes.Buffer
	failWrite bool
}

func newBufferSSEWriter(failWrite bool) *bufferSSEWriter {
	return &bufferSSEWriter{header: make(http.Header), failWrite: failWrite}
}

func (w *bufferSSEWriter) Header() http.Header {
	return w.header
}

func (w *bufferSSEWriter) WriteHeader(status int) {
	w.status = status
}

func (w *bufferSSEWriter) Write(data []byte) (int, error) {
	if w.failWrite {
		return 0, fmt.Errorf("forced write error")
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

func (w *bufferSSEWriter) Flush() {}

func (w *bufferSSEWriter) String() string {
	return w.body.String()
}
