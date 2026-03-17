package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 15 * time.Second

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = "http://localhost:4747"
	}

	return &Client{
		baseURL: strings.TrimRight(base, "/"),
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

func (c *Client) ListSessions(ctx context.Context) ([]Session, error) {
	var sessions []Session
	if err := c.doJSON(ctx, http.MethodGet, "/sessions", nil, &sessions); err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	return sessions, nil
}

func (c *Client) GetSession(ctx context.Context, sessionID string) (*SessionWithAnnotations, error) {
	var session SessionWithAnnotations
	path := fmt.Sprintf("/sessions/%s", url.PathEscape(sessionID))
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &session); err != nil {
		return nil, fmt.Errorf("getting session %q: %w", sessionID, err)
	}
	return &session, nil
}

func (c *Client) GetPending(ctx context.Context, sessionID string) (*PendingResponse, error) {
	var pending PendingResponse
	path := "/pending"
	if sessionID != "" {
		path = fmt.Sprintf("/sessions/%s/pending", url.PathEscape(sessionID))
	}

	if err := c.doJSON(ctx, http.MethodGet, path, nil, &pending); err != nil {
		return nil, fmt.Errorf("getting pending annotations: %w", err)
	}
	return &pending, nil
}

func (c *Client) Acknowledge(ctx context.Context, annotationID string) error {
	path := fmt.Sprintf("/annotations/%s", url.PathEscape(annotationID))
	if err := c.doJSON(ctx, http.MethodPatch, path, map[string]any{"status": "acknowledged"}, nil); err != nil {
		return fmt.Errorf("acknowledging annotation %q: %w", annotationID, err)
	}
	return nil
}

func (c *Client) Resolve(ctx context.Context, annotationID, summary string) error {
	path := fmt.Sprintf("/annotations/%s", url.PathEscape(annotationID))
	body := map[string]any{
		"status":     "resolved",
		"resolvedBy": "agent",
	}
	if err := c.doJSON(ctx, http.MethodPatch, path, body, nil); err != nil {
		return fmt.Errorf("resolving annotation %q: %w", annotationID, err)
	}

	if strings.TrimSpace(summary) == "" {
		return nil
	}

	message := fmt.Sprintf("Resolved: %s", strings.TrimSpace(summary))
	if err := c.addThreadMessage(ctx, annotationID, "agent", message); err != nil {
		return fmt.Errorf("adding resolution summary for annotation %q: %w", annotationID, err)
	}

	return nil
}

func (c *Client) Dismiss(ctx context.Context, annotationID, reason string) error {
	path := fmt.Sprintf("/annotations/%s", url.PathEscape(annotationID))
	body := map[string]any{
		"status":     "dismissed",
		"resolvedBy": "agent",
	}
	if err := c.doJSON(ctx, http.MethodPatch, path, body, nil); err != nil {
		return fmt.Errorf("dismissing annotation %q: %w", annotationID, err)
	}

	message := fmt.Sprintf("Dismissed: %s", strings.TrimSpace(reason))
	if err := c.addThreadMessage(ctx, annotationID, "agent", message); err != nil {
		return fmt.Errorf("adding dismissal message for annotation %q: %w", annotationID, err)
	}

	return nil
}

func (c *Client) Reply(ctx context.Context, annotationID, message string) error {
	if err := c.addThreadMessage(ctx, annotationID, "agent", message); err != nil {
		return fmt.Errorf("replying to annotation %q: %w", annotationID, err)
	}
	return nil
}

func (c *Client) addThreadMessage(ctx context.Context, annotationID, role, content string) error {
	path := fmt.Sprintf("/annotations/%s/thread", url.PathEscape(annotationID))
	body := map[string]any{
		"role":    role,
		"content": content,
	}
	if err := c.doJSON(ctx, http.MethodPost, path, body, nil); err != nil {
		return err
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, target any) error {
	requestBody, err := marshalBody(body)
	if err != nil {
		return fmt.Errorf("marshaling request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, requestBody)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		payload, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("http %d (failed to read error body: %v)", resp.StatusCode, readErr)
		}
		message := strings.TrimSpace(string(payload))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("http %d: %s", resp.StatusCode, message)
	}

	if target == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}

func marshalBody(body any) (io.Reader, error) {
	if body == nil {
		return nil, nil
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(payload), nil
}
