package http

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/router/config"
	"github.com/benjitaylor/agentation/cli/internal/router/model"
	routerpkg "github.com/benjitaylor/agentation/cli/internal/router/router"
	"github.com/benjitaylor/agentation/cli/internal/router/store"
)

func TestRegisterAndListSessions(t *testing.T) {
	cfg := testConfig()
	registry := store.NewRegistry(time.Minute)
	forwarder := routerpkg.NewForwarder(time.Second)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	httpServer := NewServer(cfg, logger, registry, forwarder)
	testServer := httptest.NewServer(httpServer.Handler)
	defer testServer.Close()

	registerPayload := model.RegisterRequest{
		SessionID: "session-a",
		ProjectID: "project-a",
		Root:      "/repo/a",
		Endpoint:  "http://127.0.0.1:9001",
	}
	registerBody, error := json.Marshal(registerPayload)
	if error != nil {
		t.Fatalf("json.Marshal returned error: %v", error)
	}

	response, error := nethttp.Post(testServer.URL+"/register", "application/json", bytes.NewReader(registerBody))
	if error != nil {
		t.Fatalf("register request returned error: %v", error)
	}
	defer response.Body.Close()
	if response.StatusCode != nethttp.StatusOK {
		t.Fatalf("register status %d, want %d", response.StatusCode, nethttp.StatusOK)
	}

	sessionsResponse, error := nethttp.Get(testServer.URL + "/sessions")
	if error != nil {
		t.Fatalf("sessions request returned error: %v", error)
	}
	defer sessionsResponse.Body.Close()
	if sessionsResponse.StatusCode != nethttp.StatusOK {
		t.Fatalf("sessions status %d, want %d", sessionsResponse.StatusCode, nethttp.StatusOK)
	}

	var payload map[string][]model.Session
	if error := json.NewDecoder(sessionsResponse.Body).Decode(&payload); error != nil {
		t.Fatalf("decoding sessions payload returned error: %v", error)
	}
	if len(payload["sessions"]) != 1 {
		t.Fatalf("sessions length %d, want %d", len(payload["sessions"]), 1)
	}
}

func TestOpenRoutesByProjectID(t *testing.T) {
	callsA := 0
	targetA := httptest.NewServer(nethttp.HandlerFunc(func(writer nethttp.ResponseWriter, request *nethttp.Request) {
		if request.URL.Path == "/open" {
			callsA++
		}
		writer.WriteHeader(nethttp.StatusNoContent)
	}))
	defer targetA.Close()

	callsB := 0
	targetB := httptest.NewServer(nethttp.HandlerFunc(func(writer nethttp.ResponseWriter, request *nethttp.Request) {
		if request.URL.Path == "/open" {
			callsB++
		}
		writer.WriteHeader(nethttp.StatusNoContent)
	}))
	defer targetB.Close()

	cfg := testConfig()
	registry := store.NewRegistry(time.Minute)
	registry.Register(model.RegisterRequest{SessionID: "session-a", ProjectID: "project-a", Root: "/repo/a", Endpoint: targetA.URL})
	registry.Register(model.RegisterRequest{SessionID: "session-b", ProjectID: "project-b", Root: "/repo/b", Endpoint: targetB.URL})
	forwarder := routerpkg.NewForwarder(time.Second)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	httpServer := NewServer(cfg, logger, registry, forwarder)
	testServer := httptest.NewServer(httpServer.Handler)
	defer testServer.Close()

	response, error := nethttp.Get(testServer.URL + "/open?projectId=project-b&path=src%2FButton.tsx&line=10&column=2")
	if error != nil {
		t.Fatalf("open request returned error: %v", error)
	}
	defer response.Body.Close()
	if response.StatusCode != nethttp.StatusNoContent {
		t.Fatalf("open status %d, want %d", response.StatusCode, nethttp.StatusNoContent)
	}
	if callsA != 0 {
		t.Fatalf("target A received %d open calls, want %d", callsA, 0)
	}
	if callsB != 1 {
		t.Fatalf("target B received %d open calls, want %d", callsB, 1)
	}
}

func TestOpenAmbiguousReturnsConflict(t *testing.T) {
	cfg := testConfig()
	registry := store.NewRegistry(time.Minute)
	registry.Register(model.RegisterRequest{SessionID: "session-a", ProjectID: "project-a", Root: "/repo/a", Endpoint: "http://127.0.0.1:9011"})
	registry.Register(model.RegisterRequest{SessionID: "session-b", ProjectID: "project-b", Root: "/repo/b", Endpoint: "http://127.0.0.1:9012"})
	forwarder := routerpkg.NewForwarder(time.Second)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	httpServer := NewServer(cfg, logger, registry, forwarder)
	testServer := httptest.NewServer(httpServer.Handler)
	defer testServer.Close()

	response, error := nethttp.Get(testServer.URL + "/open?path=src%2FButton.tsx")
	if error != nil {
		t.Fatalf("open request returned error: %v", error)
	}
	defer response.Body.Close()
	if response.StatusCode != nethttp.StatusConflict {
		t.Fatalf("open status %d, want %d", response.StatusCode, nethttp.StatusConflict)
	}
}

func TestOpenRejectsTraversalPath(t *testing.T) {
	cfg := testConfig()
	registry := store.NewRegistry(time.Minute)
	registry.Register(model.RegisterRequest{SessionID: "session-a", ProjectID: "project-a", Root: "/repo/a", Endpoint: "http://127.0.0.1:9011"})
	forwarder := routerpkg.NewForwarder(time.Second)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	httpServer := NewServer(cfg, logger, registry, forwarder)
	testServer := httptest.NewServer(httpServer.Handler)
	defer testServer.Close()

	response, error := nethttp.Get(testServer.URL + "/open?projectId=project-a&path=..%2F..%2Fsecret.txt")
	if error != nil {
		t.Fatalf("open request returned error: %v", error)
	}
	defer response.Body.Close()
	if response.StatusCode != nethttp.StatusBadRequest {
		t.Fatalf("open status %d, want %d", response.StatusCode, nethttp.StatusBadRequest)
	}
}

func TestRegisterRequiresTokenWhenConfigured(t *testing.T) {
	cfg := testConfig()
	cfg.AuthToken = "secret-token"
	registry := store.NewRegistry(time.Minute)
	forwarder := routerpkg.NewForwarder(time.Second)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	httpServer := NewServer(cfg, logger, registry, forwarder)
	testServer := httptest.NewServer(httpServer.Handler)
	defer testServer.Close()

	registerPayload := model.RegisterRequest{
		SessionID: "session-a",
		ProjectID: "project-a",
		Root:      "/repo/a",
		Endpoint:  "http://127.0.0.1:9001",
	}
	registerBody, error := json.Marshal(registerPayload)
	if error != nil {
		t.Fatalf("json.Marshal returned error: %v", error)
	}

	response, error := nethttp.Post(testServer.URL+"/register", "application/json", bytes.NewReader(registerBody))
	if error != nil {
		t.Fatalf("register request returned error: %v", error)
	}
	defer response.Body.Close()
	if response.StatusCode != nethttp.StatusUnauthorized {
		t.Fatalf("register status %d, want %d", response.StatusCode, nethttp.StatusUnauthorized)
	}
}

func testConfig() config.Config {
	return config.Config{
		Address:            "127.0.0.1:0",
		RequestBodyLimit:   1024 * 1024,
		ForwardTimeout:     time.Second,
		ReadTimeout:        time.Second,
		WriteTimeout:       time.Second,
		ReadHeaderTimeout:  time.Second,
		IdleTimeout:        time.Second,
		SessionStaleAfter:  time.Minute,
		AllowAbsolutePaths: false,
		EnforceRootBounds:  true,
	}
}
