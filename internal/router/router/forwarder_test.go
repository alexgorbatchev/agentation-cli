package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/router/model"
)

func TestForwardPing(t *testing.T) {
	called := false
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/ping" {
			t.Fatalf("received path %q, want %q", request.URL.Path, "/ping")
		}
		called = true
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer testServer.Close()

	forwarder := NewForwarder(time.Second)
	error := forwarder.ForwardPing(context.Background(), model.Session{Endpoint: testServer.URL})
	if error != nil {
		t.Fatalf("ForwardPing returned error: %v", error)
	}
	if !called {
		t.Fatal("ForwardPing did not call the target endpoint")
	}
}

func TestForwardOpen(t *testing.T) {
	called := false
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/open" {
			t.Fatalf("received path %q, want %q", request.URL.Path, "/open")
		}
		if request.URL.Query().Get("path") != "src/Button.tsx" {
			t.Fatalf("received path query %q, want %q", request.URL.Query().Get("path"), "src/Button.tsx")
		}
		if request.URL.Query().Get("line") != "42" {
			t.Fatalf("received line query %q, want %q", request.URL.Query().Get("line"), "42")
		}
		if request.URL.Query().Get("column") != "8" {
			t.Fatalf("received column query %q, want %q", request.URL.Query().Get("column"), "8")
		}
		called = true
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer testServer.Close()

	forwarder := NewForwarder(time.Second)
	error := forwarder.ForwardOpen(context.Background(), model.Session{Endpoint: testServer.URL}, model.OpenRequest{
		Path:   "src/Button.tsx",
		Line:   42,
		Column: 8,
	})
	if error != nil {
		t.Fatalf("ForwardOpen returned error: %v", error)
	}
	if !called {
		t.Fatal("ForwardOpen did not call the target endpoint")
	}
}

func TestBuildTargetURL(t *testing.T) {
	targetURL, error := buildTargetURL("http://127.0.0.1:8788/base", "/open", nil)
	if error != nil {
		t.Fatalf("buildTargetURL returned error: %v", error)
	}
	if targetURL != "http://127.0.0.1:8788/open" {
		t.Fatalf("buildTargetURL returned %q, want %q", targetURL, "http://127.0.0.1:8788/open")
	}
}
