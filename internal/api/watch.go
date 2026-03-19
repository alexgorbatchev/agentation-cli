package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBatchWindow  = 10 * time.Second
	defaultWatchTimeout = 300 * time.Second
	maxBatchWindow      = 60 * time.Second
	maxWatchTimeout     = 300 * time.Second
)

type afsEvent struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId"`
	Sequence  int             `json:"sequence"`
	Payload   json.RawMessage `json:"payload"`
}

func (c *Client) Watch(ctx context.Context, opts WatchOptions) (*WatchOutput, error) {
	batchWindow := clampDuration(opts.BatchWindow, defaultBatchWindow, time.Second, maxBatchWindow)
	watchTimeout := clampDuration(opts.Timeout, defaultWatchTimeout, time.Second, maxWatchTimeout)

	pending, err := c.GetPending(ctx, opts.SessionID, opts.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("draining pending annotations before watch: %w", err)
	}
	if pending.Count > 0 {
		sessions := uniqueSessions(pending.Annotations)
		return &WatchOutput{
			Timeout:     false,
			Count:       pending.Count,
			Sessions:    sessions,
			Annotations: pending.Annotations,
		}, nil
	}

	watchCtx, cancel := context.WithTimeout(ctx, watchTimeout)
	defer cancel()

	events := make(chan Annotation, 32)
	errs := make(chan error, 1)
	go c.streamAnnotations(watchCtx, opts.SessionID, opts.ProjectID, events, errs)

	collected := make(map[string]Annotation)
	order := make([]string, 0)
	var batchTimer *time.Timer
	var batchDone <-chan time.Time

	for {
		select {
		case ann := <-events:
			if ann.ID == "" {
				continue
			}
			if _, exists := collected[ann.ID]; !exists {
				order = append(order, ann.ID)
			}
			collected[ann.ID] = ann

			if batchTimer == nil {
				batchTimer = time.NewTimer(batchWindow)
				batchDone = batchTimer.C
			}

		case <-batchDone:
			return buildWatchOutput(collected, order), nil

		case err := <-errs:
			if len(collected) > 0 {
				return buildWatchOutput(collected, order), nil
			}
			if err == nil {
				continue
			}
			return nil, fmt.Errorf("watch stream failed: %w", err)

		case <-watchCtx.Done():
			if batchTimer != nil {
				batchTimer.Stop()
			}
			if len(collected) > 0 {
				return buildWatchOutput(collected, order), nil
			}
			if errors.Is(watchCtx.Err(), context.DeadlineExceeded) {
				return &WatchOutput{
					Timeout: true,
					Message: fmt.Sprintf("No new annotations within %d seconds", int(watchTimeout.Seconds())),
				}, nil
			}
			return nil, fmt.Errorf("watch canceled: %w", watchCtx.Err())
		}
	}
}

func (c *Client) streamAnnotations(ctx context.Context, sessionID, projectID string, out chan<- Annotation, errs chan<- error) {
	ssePath := "/events?agent=true"
	trimmedSessionID := strings.TrimSpace(sessionID)
	trimmedProjectID := strings.TrimSpace(projectID)
	if trimmedSessionID != "" {
		ssePath = fmt.Sprintf("/sessions/%s/events?agent=true", url.PathEscape(trimmedSessionID))
	} else if trimmedProjectID != "" {
		ssePath = fmt.Sprintf("/events?agent=true&projectId=%s", url.QueryEscape(trimmedProjectID))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+ssePath, nil)
	if err != nil {
		errs <- fmt.Errorf("creating watch request: %w", err)
		return
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			errs <- nil
			return
		}
		errs <- fmt.Errorf("opening watch stream: %w", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errs <- fmt.Errorf("watch endpoint returned http %d", resp.StatusCode)
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	dataLines := make([]string, 0)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if len(dataLines) > 0 {
				c.handleEventPayload(strings.Join(dataLines, "\n"), trimmedSessionID, out)
				dataLines = dataLines[:0]
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			continue
		}
		if after, ok := strings.CutPrefix(line, "data:"); ok {
			data := strings.TrimSpace(after)
			dataLines = append(dataLines, data)
		}
	}

	if scannerErr := scanner.Err(); scannerErr != nil {
		if errors.Is(scannerErr, context.Canceled) || errors.Is(scannerErr, context.DeadlineExceeded) {
			errs <- nil
			return
		}
		errs <- fmt.Errorf("reading watch stream: %w", scannerErr)
		return
	}

	if ctx.Err() != nil {
		errs <- nil
		return
	}

	errs <- errors.New("watch stream closed unexpectedly")
}

func (c *Client) handleEventPayload(payload, sessionID string, out chan<- Annotation) {
	var event afsEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return
	}

	if event.Sequence == 0 {
		return
	}
	if sessionID != "" && event.SessionID != sessionID {
		return
	}

	switch event.Type {
	case "annotation.created":
		var ann Annotation
		if err := json.Unmarshal(event.Payload, &ann); err != nil {
			return
		}
		if ann.SessionID == "" {
			ann.SessionID = event.SessionID
		}
		out <- ann

	case "thread.message":
		var ann Annotation
		if err := json.Unmarshal(event.Payload, &ann); err != nil {
			return
		}
		if len(ann.Thread) == 0 {
			return
		}
		last := ann.Thread[len(ann.Thread)-1]
		if last.Role != "human" {
			return
		}
		if ann.SessionID == "" {
			ann.SessionID = event.SessionID
		}
		out <- ann
	}
}

func buildWatchOutput(collected map[string]Annotation, order []string) *WatchOutput {
	annotations := make([]Annotation, 0, len(order))
	for _, id := range order {
		annotations = append(annotations, collected[id])
	}

	return &WatchOutput{
		Timeout:     false,
		Count:       len(annotations),
		Sessions:    uniqueSessions(annotations),
		Annotations: annotations,
	}
}

func uniqueSessions(annotations []Annotation) []string {
	seen := make(map[string]struct{})
	sessions := make([]string, 0)
	for _, ann := range annotations {
		if ann.SessionID == "" {
			continue
		}
		if _, exists := seen[ann.SessionID]; exists {
			continue
		}
		seen[ann.SessionID] = struct{}{}
		sessions = append(sessions, ann.SessionID)
	}
	return sessions
}

func clampDuration(value, fallback, minValue, maxValue time.Duration) time.Duration {
	if value <= 0 {
		value = fallback
	}
	if value < minValue {
		value = minValue
	}
	if value > maxValue {
		value = maxValue
	}
	return value
}
