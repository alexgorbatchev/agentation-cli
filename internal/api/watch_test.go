package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWatchReturnsPendingImmediately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/pending" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"count":1,"annotations":[{"id":"a1","sessionId":"s1","comment":"Fix button","element":"button","elementPath":"body > button"}]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	output, err := client.Watch(context.Background(), WatchOptions{})
	if err != nil {
		t.Fatalf("Watch returned error: %v", err)
	}

	if output.Timeout {
		t.Fatal("expected non-timeout output")
	}
	if output.Count != 1 {
		t.Fatalf("output.Count = %d, want 1", output.Count)
	}
	if len(output.Sessions) != 1 || output.Sessions[0] != "s1" {
		t.Fatalf("output.Sessions = %#v, want [\"s1\"]", output.Sessions)
	}
}

func TestWatchCollectsSSEAnnotations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/pending":
			writer.Header().Set("Content-Type", "application/json")
			fmt.Fprint(writer, `{"count":0,"annotations":[]}`)
			return
		case "/events":
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.Header().Set("Cache-Control", "no-cache")

			flusher, ok := writer.(http.Flusher)
			if !ok {
				t.Fatal("response writer does not support flushing")
			}

			fmt.Fprint(writer, ": connected\n\n")
			fmt.Fprint(writer, `data: {"type":"annotation.created","sessionId":"s1","sequence":0,"payload":{"id":"ignored","sessionId":"s1","comment":"old"}}`+"\n\n")
			flusher.Flush()

			time.Sleep(20 * time.Millisecond)
			fmt.Fprint(writer, `data: {"type":"annotation.created","sessionId":"s1","sequence":1,"payload":{"id":"a1","sessionId":"s1","comment":"Fix spacing","element":"button","elementPath":"body > button"}}`+"\n\n")
			flusher.Flush()

			time.Sleep(20 * time.Millisecond)
			fmt.Fprint(writer, `data: {"type":"thread.message","sessionId":"s2","sequence":2,"payload":{"id":"a2","sessionId":"s2","comment":"Need follow-up","element":"div","elementPath":"body > div","thread":[{"id":"m1","role":"human","content":"Please also change color","timestamp":1}]}}`+"\n\n")
			flusher.Flush()

			<-request.Context().Done()
			return
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	output, err := client.Watch(context.Background(), WatchOptions{
		BatchWindow: 80 * time.Millisecond,
		Timeout:     2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Watch returned error: %v", err)
	}

	if output.Timeout {
		t.Fatal("expected non-timeout output")
	}
	if output.Count != 2 {
		t.Fatalf("output.Count = %d, want 2", output.Count)
	}
	if output.Annotations[0].ID != "a1" {
		t.Fatalf("first annotation ID = %s, want a1", output.Annotations[0].ID)
	}
	if output.Annotations[1].ID != "a2" {
		t.Fatalf("second annotation ID = %s, want a2", output.Annotations[1].ID)
	}
}
