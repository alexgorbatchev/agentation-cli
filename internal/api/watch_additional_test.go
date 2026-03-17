package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWatchTimeoutWhenNoEvents(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/pending":
			_, _ = writer.Write([]byte(`{"count":0,"annotations":[]}`))
		case "/events":
			writer.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := writer.(http.Flusher)
			if !ok {
				t.Fatal("missing flusher")
			}
			_, _ = writer.Write([]byte(": connected\n\n"))
			flusher.Flush()
			<-request.Context().Done()
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer testServer.Close()

	client := NewClient(testServer.URL)
	output, err := client.Watch(context.Background(), WatchOptions{
		Timeout: 1100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Watch returned error: %v", err)
	}
	if !output.Timeout {
		t.Fatal("expected timeout output")
	}
	if !strings.Contains(output.Message, "No new annotations") {
		t.Fatalf("unexpected timeout message: %q", output.Message)
	}
}

func TestWatchReturnsErrorWhenPendingFails(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte("boom"))
	}))
	defer testServer.Close()

	client := NewClient(testServer.URL)
	_, err := client.Watch(context.Background(), WatchOptions{Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "draining pending annotations") {
		t.Fatalf("expected pending drain error, got %v", err)
	}
}

func TestWatchUsesSessionEventsPath(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/sessions/s1/pending":
			_, _ = writer.Write([]byte(`{"count":0,"annotations":[]}`))
		case "/sessions/s1/events":
			writer.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := writer.(http.Flusher)
			if !ok {
				t.Fatal("missing flusher")
			}
			_, _ = writer.Write([]byte(`data: {"type":"annotation.created","sessionId":"s1","sequence":1,"payload":{"id":"a1","comment":"Fix","element":"button","elementPath":"body > button"}}` + "\n\n"))
			flusher.Flush()
			<-request.Context().Done()
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer testServer.Close()

	client := NewClient(testServer.URL)
	output, err := client.Watch(context.Background(), WatchOptions{
		SessionID:   "s1",
		BatchWindow: 50 * time.Millisecond,
		Timeout:     2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Watch returned error: %v", err)
	}
	if output.Count != 1 || output.Annotations[0].SessionID != "s1" {
		t.Fatalf("unexpected output: %#v", output)
	}
}

func TestHandleEventPayloadFilters(t *testing.T) {
	client := NewClient("http://localhost:4747")
	out := make(chan Annotation, 10)

	client.handleEventPayload("not-json", "", out)
	client.handleEventPayload(`{"type":"annotation.created","sessionId":"s1","sequence":0,"payload":{"id":"a0"}}`, "", out)
	client.handleEventPayload(`{"type":"annotation.created","sessionId":"s2","sequence":1,"payload":{"id":"a1"}}`, "s1", out)
	client.handleEventPayload(`{"type":"thread.message","sessionId":"s1","sequence":1,"payload":{"id":"a2","thread":[]}}`, "", out)
	client.handleEventPayload(`{"type":"thread.message","sessionId":"s1","sequence":1,"payload":{"id":"a3","thread":[{"role":"agent","content":"x","timestamp":1}]}}`, "", out)
	client.handleEventPayload(`{"type":"unknown","sessionId":"s1","sequence":1,"payload":{"id":"a4"}}`, "", out)

	if len(out) != 0 {
		t.Fatalf("expected no forwarded annotations, got %d", len(out))
	}

	client.handleEventPayload(`{"type":"annotation.created","sessionId":"s1","sequence":2,"payload":{"id":"a5","comment":"Fix","element":"button","elementPath":"body > button"}}`, "", out)
	client.handleEventPayload(`{"type":"thread.message","sessionId":"s1","sequence":2,"payload":{"id":"a6","thread":[{"role":"human","content":"help","timestamp":1}]}}`, "", out)

	if len(out) != 2 {
		t.Fatalf("expected 2 forwarded annotations, got %d", len(out))
	}
}

func TestWatchReturnsCollectedOnStreamError(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/pending":
			_, _ = writer.Write([]byte(`{"count":0,"annotations":[]}`))
		case "/events":
			writer.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := writer.(http.Flusher)
			if !ok {
				t.Fatal("missing flusher")
			}
			_, _ = writer.Write([]byte(`data: {"type":"annotation.created","sessionId":"s1","sequence":1,"payload":{"id":"a1","comment":"Fix","element":"button","elementPath":"body > button"}}` + "\n\n"))
			flusher.Flush()
			return
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer testServer.Close()

	client := NewClient(testServer.URL)
	output, err := client.Watch(context.Background(), WatchOptions{
		BatchWindow: 2 * time.Second,
		Timeout:     3 * time.Second,
	})
	if err != nil {
		t.Fatalf("Watch returned error: %v", err)
	}
	if output.Count != 1 {
		t.Fatalf("output.Count = %d, want 1", output.Count)
	}
}

func TestWatchReturnsCanceledError(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/pending":
			_, _ = writer.Write([]byte(`{"count":0,"annotations":[]}`))
		case "/events":
			writer.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := writer.(http.Flusher)
			if !ok {
				t.Fatal("missing flusher")
			}
			_, _ = writer.Write([]byte(": connected\n\n"))
			flusher.Flush()
			<-request.Context().Done()
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer testServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(120 * time.Millisecond)
		cancel()
	}()

	client := NewClient(testServer.URL)
	_, err := client.Watch(ctx, WatchOptions{Timeout: 10 * time.Second})
	if err == nil || !strings.Contains(err.Error(), "watch canceled") {
		t.Fatalf("expected canceled error, got %v", err)
	}
}

func TestStreamAnnotationsDirectPaths(t *testing.T) {
	client := NewClient("http://[::1")
	errCh := make(chan error, 1)
	out := make(chan Annotation, 1)
	client.streamAnnotations(context.Background(), "", out, errCh)
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "creating watch request") {
		t.Fatalf("expected request creation error, got %v", err)
	}

	notOKServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer notOKServer.Close()

	client = NewClient(notOKServer.URL)
	errCh = make(chan error, 1)
	client.streamAnnotations(context.Background(), "", out, errCh)
	err = <-errCh
	if err == nil || !strings.Contains(err.Error(), "http 503") {
		t.Fatalf("expected non-200 error, got %v", err)
	}

	closedServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if ok {
			flusher.Flush()
		}
	}))
	defer closedServer.Close()

	client = NewClient(closedServer.URL)
	errCh = make(chan error, 1)
	client.streamAnnotations(context.Background(), "", out, errCh)
	err = <-errCh
	if err == nil || !strings.Contains(err.Error(), "closed unexpectedly") {
		t.Fatalf("expected unexpected close error, got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	errCh = make(chan error, 1)
	client.streamAnnotations(ctx, "", out, errCh)
	err = <-errCh
	if err != nil {
		t.Fatalf("expected nil error on canceled context, got %v", err)
	}
}

func TestClampDurationAndUniqueSessions(t *testing.T) {
	if got := clampDuration(0, 10*time.Second, time.Second, 60*time.Second); got != 10*time.Second {
		t.Fatalf("clampDuration fallback = %v", got)
	}
	if got := clampDuration(200*time.Millisecond, 10*time.Second, time.Second, 60*time.Second); got != time.Second {
		t.Fatalf("clampDuration min = %v", got)
	}
	if got := clampDuration(120*time.Second, 10*time.Second, time.Second, 60*time.Second); got != 60*time.Second {
		t.Fatalf("clampDuration max = %v", got)
	}

	sessions := uniqueSessions([]Annotation{{SessionID: "s1"}, {SessionID: "s2"}, {SessionID: "s1"}, {SessionID: ""}})
	if len(sessions) != 2 || sessions[0] != "s1" || sessions[1] != "s2" {
		t.Fatalf("uniqueSessions = %#v", sessions)
	}
}
