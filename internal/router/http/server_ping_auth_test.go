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

func TestPingRequiresTokenWhenConfigured(t *testing.T) {
	cfg := testConfig()
	cfg.AuthToken = "secret-token"
	registry := store.NewRegistry(time.Minute)
	registry.Register(model.RegisterRequest{
		SessionID: "session-a",
		ProjectID: "project-a",
		Root:      "/repo/a",
		Endpoint:  "http://127.0.0.1:9011",
	})
	forwarder := routerpkg.NewForwarder(time.Second)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	httpServer := NewServer(cfg, logger, registry, forwarder)
	testServer := httptest.NewServer(httpServer.Handler)
	defer testServer.Close()

	response, error := nethttp.Get(testServer.URL + "/ping?projectId=project-a")
	if error != nil {
		t.Fatalf("ping request returned error: %v", error)
	}
	defer response.Body.Close()

	if response.StatusCode != nethttp.StatusUnauthorized {
		t.Fatalf("ping status %d, want %d", response.StatusCode, nethttp.StatusUnauthorized)
	}
}

func TestPingAllowsAuthorizedRequestWhenTokenConfigured(t *testing.T) {
	calls := 0
	target := httptest.NewServer(nethttp.HandlerFunc(func(writer nethttp.ResponseWriter, request *nethttp.Request) {
		if request.URL.Path == "/ping" {
			calls++
		}
		writer.WriteHeader(nethttp.StatusNoContent)
	}))
	defer target.Close()

	cfg := testConfig()
	cfg.AuthToken = "secret-token"
	registry := store.NewRegistry(time.Minute)
	registry.Register(model.RegisterRequest{
		SessionID: "session-a",
		ProjectID: "project-a",
		Root:      "/repo/a",
		Endpoint:  target.URL,
	})
	forwarder := routerpkg.NewForwarder(time.Second)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	httpServer := NewServer(cfg, logger, registry, forwarder)
	testServer := httptest.NewServer(httpServer.Handler)
	defer testServer.Close()

	request, error := nethttp.NewRequest(nethttp.MethodGet, testServer.URL+"/ping?projectId=project-a", nil)
	if error != nil {
		t.Fatalf("NewRequest returned error: %v", error)
	}
	request.Header.Set("X-Agentation-Token", "secret-token")

	response, error := nethttp.DefaultClient.Do(request)
	if error != nil {
		t.Fatalf("ping request returned error: %v", error)
	}
	defer response.Body.Close()

	if response.StatusCode != nethttp.StatusNoContent {
		t.Fatalf("ping status %d, want %d", response.StatusCode, nethttp.StatusNoContent)
	}
	if calls != 1 {
		t.Fatalf("target received %d ping calls, want %d", calls, 1)
	}
}
