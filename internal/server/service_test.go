package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionAndPendingFlow(t *testing.T) {
	t.Setenv("AGENTATION_STORE", "memory")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := NewService("127.0.0.1:0", logger)
	testServer := httptest.NewServer(service.httpServer.Handler)
	defer testServer.Close()

	sessionID := createSession(t, testServer.URL, "http://example.com")

	annotationID := createAnnotation(t, testServer.URL, sessionID, map[string]any{
		"x":           10,
		"y":           20,
		"comment":     "Fix this button",
		"element":     "button",
		"elementPath": "body > button",
		"timestamp":   123,
	})

	pending := getPending(t, testServer.URL, "/pending")
	if pending.Count != 1 {
		t.Fatalf("pending.Count = %d, want 1", pending.Count)
	}

	patchBody := map[string]any{"status": "acknowledged"}
	callJSON(t, http.MethodPatch, testServer.URL+"/annotations/"+annotationID, patchBody, nil, http.StatusOK)

	pending = getPending(t, testServer.URL, "/pending")
	if pending.Count != 0 {
		t.Fatalf("pending.Count after acknowledge = %d, want 0", pending.Count)
	}

	reply := map[string]any{"role": "human", "content": "Please also update hover state"}
	callJSON(t, http.MethodPost, testServer.URL+"/annotations/"+annotationID+"/thread", reply, nil, http.StatusCreated)

	pending = getPending(t, testServer.URL, "/pending")
	if pending.Count != 1 {
		t.Fatalf("pending.Count after human reply = %d, want 1", pending.Count)
	}
}

func createSession(t *testing.T, baseURL, pageURL string) string {
	t.Helper()

	var session Session
	callJSON(t, http.MethodPost, baseURL+"/sessions", map[string]any{"url": pageURL}, &session, http.StatusCreated)
	if session.ID == "" {
		t.Fatal("session.ID should not be empty")
	}
	return session.ID
}

func createAnnotation(t *testing.T, baseURL, sessionID string, body map[string]any) string {
	t.Helper()

	var annotation Annotation
	callJSON(t, http.MethodPost, baseURL+"/sessions/"+sessionID+"/annotations", body, &annotation, http.StatusCreated)
	if annotation.ID == "" {
		t.Fatal("annotation.ID should not be empty")
	}
	return annotation.ID
}

func getPending(t *testing.T, baseURL, path string) pendingResponse {
	t.Helper()

	var pending pendingResponse
	callJSON(t, http.MethodGet, baseURL+path, nil, &pending, http.StatusOK)
	return pending
}

func callJSON(t *testing.T, method, endpoint string, body any, target any, expectedStatus int) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		t.Fatalf("http.NewRequest failed: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != expectedStatus {
		content, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want %d, body = %s", response.StatusCode, expectedStatus, string(content))
	}

	if target != nil {
		if err := json.NewDecoder(response.Body).Decode(target); err != nil {
			t.Fatalf("json decode failed: %v", err)
		}
	}
}
