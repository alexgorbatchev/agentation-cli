package http

import (
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/router/model"
	routerpkg "github.com/benjitaylor/agentation/cli/internal/router/router"
	"github.com/benjitaylor/agentation/cli/internal/router/store"
)

func TestOpenRequiresTokenWhenConfigured(t *testing.T) {
	calls := 0
	target := httptest.NewServer(nethttp.HandlerFunc(func(writer nethttp.ResponseWriter, request *nethttp.Request) {
		if request.URL.Path == "/open" {
			calls++
		}
		writer.WriteHeader(nethttp.StatusNoContent)
	}))
	defer target.Close()

	cfg := testConfig()
	cfg.AuthToken = "secret-token"
	registry := store.NewRegistry(time.Minute)
	registry.Register(registerRequestForTarget(target.URL))
	forwarder := routerpkg.NewForwarder(time.Second)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	httpServer := NewServer(cfg, logger, registry, forwarder)
	testServer := httptest.NewServer(httpServer.Handler)
	defer testServer.Close()

	response, error := nethttp.Get(testServer.URL + "/open?projectId=project-a&path=src%2FButton.tsx")
	if error != nil {
		t.Fatalf("open request returned error: %v", error)
	}
	defer response.Body.Close()

	if response.StatusCode != nethttp.StatusUnauthorized {
		t.Fatalf("open status %d, want %d", response.StatusCode, nethttp.StatusUnauthorized)
	}
	if calls != 0 {
		t.Fatalf("target received %d open calls, want %d", calls, 0)
	}
}

func TestOpenAllowsAuthorizedRequestWhenTokenConfigured(t *testing.T) {
	calls := 0
	target := httptest.NewServer(nethttp.HandlerFunc(func(writer nethttp.ResponseWriter, request *nethttp.Request) {
		if request.URL.Path == "/open" {
			calls++
		}
		writer.WriteHeader(nethttp.StatusNoContent)
	}))
	defer target.Close()

	cfg := testConfig()
	cfg.AuthToken = "secret-token"
	registry := store.NewRegistry(time.Minute)
	registry.Register(registerRequestForTarget(target.URL))
	forwarder := routerpkg.NewForwarder(time.Second)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	httpServer := NewServer(cfg, logger, registry, forwarder)
	testServer := httptest.NewServer(httpServer.Handler)
	defer testServer.Close()

	request, error := nethttp.NewRequest(nethttp.MethodGet, testServer.URL+"/open?projectId=project-a&path=src%2FButton.tsx", nil)
	if error != nil {
		t.Fatalf("nethttp.NewRequest returned error: %v", error)
	}
	request.Header.Set("X-Agentation-Token", "secret-token")

	response, error := nethttp.DefaultClient.Do(request)
	if error != nil {
		t.Fatalf("open request returned error: %v", error)
	}
	defer response.Body.Close()

	if response.StatusCode != nethttp.StatusNoContent {
		t.Fatalf("open status %d, want %d", response.StatusCode, nethttp.StatusNoContent)
	}
	if calls != 1 {
		t.Fatalf("target received %d open calls, want %d", calls, 1)
	}
}

func registerRequestForTarget(endpoint string) model.RegisterRequest {
	return model.RegisterRequest{
		SessionID: "session-a",
		ProjectID: "project-a",
		Root:      "/repo/a",
		Endpoint:  endpoint,
	}
}
