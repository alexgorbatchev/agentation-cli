package router

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/router/model"
)

type Forwarder struct {
	client *http.Client
}

func NewForwarder(timeout time.Duration) *Forwarder {
	return &Forwarder{
		client: &http.Client{Timeout: timeout},
	}
}

func (f *Forwarder) ForwardPing(ctx context.Context, session model.Session) error {
	targetURL, error := buildTargetURL(session.Endpoint, "/ping", nil)
	if error != nil {
		return error
	}

	request, error := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if error != nil {
		return fmt.Errorf("creating ping request: %w", error)
	}

	response, error := f.client.Do(request)
	if error != nil {
		return fmt.Errorf("sending ping request: %w", error)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("ping request failed with status %d", response.StatusCode)
	}

	return nil
}

func (f *Forwarder) ForwardOpen(ctx context.Context, session model.Session, openRequest model.OpenRequest) error {
	query := url.Values{}
	query.Set("path", openRequest.Path)
	query.Set("line", strconv.Itoa(max(openRequest.Line, 1)))
	query.Set("column", strconv.Itoa(max(openRequest.Column, 1)))
	targetURL, error := buildTargetURL(session.Endpoint, "/open", query)
	if error != nil {
		return error
	}

	request, error := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if error != nil {
		return fmt.Errorf("creating open request: %w", error)
	}

	response, error := f.client.Do(request)
	if error != nil {
		return fmt.Errorf("sending open request: %w", error)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("open request failed with status %d", response.StatusCode)
	}

	return nil
}

func buildTargetURL(endpoint string, routePath string, query url.Values) (string, error) {
	trimmedEndpoint := strings.TrimSpace(endpoint)
	if trimmedEndpoint == "" {
		return "", fmt.Errorf("session endpoint is required")
	}

	baseURL, error := url.Parse(trimmedEndpoint)
	if error != nil {
		return "", fmt.Errorf("parsing session endpoint %q: %w", trimmedEndpoint, error)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return "", fmt.Errorf("session endpoint %q is invalid", trimmedEndpoint)
	}

	routeURL, error := url.Parse(routePath)
	if error != nil {
		return "", fmt.Errorf("parsing route path %q: %w", routePath, error)
	}

	resolvedURL := baseURL.ResolveReference(routeURL)
	if query != nil {
		resolvedURL.RawQuery = query.Encode()
	}

	return resolvedURL.String(), nil
}

func max(value int, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}
