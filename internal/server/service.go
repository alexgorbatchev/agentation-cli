package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const requestBodyLimit = 2 << 20

type Service struct {
	store *Store
	log   *slog.Logger

	httpServer *http.Server

	activeListeners int64
	agentListeners  int64

	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	shutdownOnce   sync.Once
}

func NewService(address string, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	service := &Service{
		store:          NewStore(),
		log:            logger,
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", service.handleHealth)
	mux.HandleFunc("GET /status", service.handleStatus)
	mux.HandleFunc("GET /sessions", service.handleListSessions)
	mux.HandleFunc("POST /sessions", service.handleCreateSession)
	mux.HandleFunc("GET /sessions/{id}", service.handleGetSession)
	mux.HandleFunc("POST /sessions/{id}/annotations", service.handleAddAnnotation)
	mux.HandleFunc("GET /sessions/{id}/pending", service.handleSessionPending)
	mux.HandleFunc("GET /sessions/{id}/events", service.handleSessionEvents)
	mux.HandleFunc("POST /sessions/{id}/action", service.handleRequestAction)
	mux.HandleFunc("GET /annotations/{id}", service.handleGetAnnotation)
	mux.HandleFunc("PATCH /annotations/{id}", service.handleUpdateAnnotation)
	mux.HandleFunc("DELETE /annotations/{id}", service.handleDeleteAnnotation)
	mux.HandleFunc("POST /annotations/{id}/thread", service.handleAddThreadMessage)
	mux.HandleFunc("GET /pending", service.handleAllPending)
	mux.HandleFunc("GET /events", service.handleGlobalEvents)

	service.httpServer = &http.Server{
		Addr:    address,
		Handler: service.withCORS(mux),
	}

	return service
}

func (s *Service) ListenAndServe() error {
	s.log.Info("agentation server listening", "address", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Service) Shutdown(ctx context.Context) error {
	s.shutdownOnce.Do(func() {
		s.shutdownCancel()
	})

	err := s.httpServer.Shutdown(ctx)
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		s.log.Warn("graceful shutdown timed out, forcing server close", "error", err)
		closeErr := s.httpServer.Close()
		if closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			return closeErr
		}
		return nil
	}

	return err
}

func (s *Service) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Last-Event-ID")
		writer.Header().Set("Access-Control-Expose-Headers", "Last-Event-ID")

		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(writer, request)
	})
}

func (s *Service) handleHealth(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"status": "ok", "mode": "local"})
}

func (s *Service) handleStatus(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"mode":               "local",
		"webhooksConfigured": false,
		"webhookCount":       0,
		"activeListeners":    atomic.LoadInt64(&s.activeListeners),
		"agentListeners":     atomic.LoadInt64(&s.agentListeners),
	})
}

func (s *Service) handleListSessions(writer http.ResponseWriter, request *http.Request) {
	projectID := strings.TrimSpace(request.URL.Query().Get("projectId"))
	sessions := s.store.ListSessions()
	if projectID != "" {
		filtered := make([]Session, 0)
		for _, session := range sessions {
			if session.ProjectID == projectID {
				filtered = append(filtered, session)
			}
		}
		sessions = filtered
	}

	writeJSON(writer, http.StatusOK, sessions)
}

func (s *Service) handleCreateSession(writer http.ResponseWriter, request *http.Request) {
	var input sessionCreateInput
	if err := decodeBody(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}

	if strings.TrimSpace(input.URL) == "" {
		writeError(writer, http.StatusBadRequest, "url is required")
		return
	}

	session := s.store.CreateSession(strings.TrimSpace(input.URL), strings.TrimSpace(input.ProjectID))
	s.log.Info("frontend session connected", "sessionId", session.ID, "url", session.URL)
	writeJSON(writer, http.StatusCreated, session)
}

func (s *Service) handleGetSession(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.PathValue("id")
	session, ok := s.store.GetSessionWithAnnotations(sessionID)
	if !ok {
		writeError(writer, http.StatusNotFound, "Session not found")
		return
	}

	s.log.Info("frontend session loaded", "sessionId", session.ID, "annotations", len(session.Annotations))
	writeJSON(writer, http.StatusOK, session)
}

func (s *Service) handleAddAnnotation(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.PathValue("id")
	var annotation Annotation
	if err := decodeBody(request, &annotation); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}

	if strings.TrimSpace(annotation.Comment) == "" || strings.TrimSpace(annotation.Element) == "" || strings.TrimSpace(annotation.ElementPath) == "" {
		writeError(writer, http.StatusBadRequest, "comment, element, and elementPath are required")
		return
	}

	created, ok := s.store.AddAnnotation(sessionID, annotation)
	if !ok {
		writeError(writer, http.StatusNotFound, "Session not found")
		return
	}

	s.log.Info("frontend annotation received", "sessionId", created.SessionID, "annotationId", created.ID, "element", created.Element)
	writeJSON(writer, http.StatusCreated, created)
}

func (s *Service) handleGetAnnotation(writer http.ResponseWriter, request *http.Request) {
	annotationID := request.PathValue("id")
	annotation, ok := s.store.GetAnnotation(annotationID)
	if !ok {
		writeError(writer, http.StatusNotFound, "Annotation not found")
		return
	}
	writeJSON(writer, http.StatusOK, annotation)
}

func (s *Service) handleUpdateAnnotation(writer http.ResponseWriter, request *http.Request) {
	annotationID := request.PathValue("id")
	patch := make(map[string]any)
	if err := decodeBody(request, &patch); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}

	annotation, ok := s.store.UpdateAnnotation(annotationID, patch)
	if !ok {
		writeError(writer, http.StatusNotFound, "Annotation not found")
		return
	}

	writeJSON(writer, http.StatusOK, annotation)
}

func (s *Service) handleDeleteAnnotation(writer http.ResponseWriter, request *http.Request) {
	annotationID := request.PathValue("id")
	_, ok := s.store.DeleteAnnotation(annotationID)
	if !ok {
		writeError(writer, http.StatusNotFound, "Annotation not found")
		return
	}

	writeJSON(writer, http.StatusOK, map[string]any{"deleted": true, "annotationId": annotationID})
}

func (s *Service) handleAddThreadMessage(writer http.ResponseWriter, request *http.Request) {
	annotationID := request.PathValue("id")
	var input threadInput
	if err := decodeBody(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}

	if input.Role != "human" && input.Role != "agent" {
		writeError(writer, http.StatusBadRequest, "role must be 'human' or 'agent'")
		return
	}
	if strings.TrimSpace(input.Content) == "" {
		writeError(writer, http.StatusBadRequest, "content is required")
		return
	}

	annotation, ok := s.store.AddThreadMessage(annotationID, input.Role, strings.TrimSpace(input.Content))
	if !ok {
		writeError(writer, http.StatusNotFound, "Annotation not found")
		return
	}

	writeJSON(writer, http.StatusCreated, annotation)
}

func (s *Service) handleSessionPending(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.PathValue("id")
	_, ok := s.store.GetSession(sessionID)
	if !ok {
		writeError(writer, http.StatusNotFound, "Session not found")
		return
	}

	pending := s.store.GetAnnotationsNeedingAttention(sessionID)
	writeJSON(writer, http.StatusOK, pendingResponse{Count: len(pending), Annotations: pending})
}

func (s *Service) handleAllPending(writer http.ResponseWriter, request *http.Request) {
	projectID := strings.TrimSpace(request.URL.Query().Get("projectId"))

	pending := s.store.GetAllAnnotationsNeedingAttention()
	if projectID != "" {
		pending = s.store.GetAllAnnotationsNeedingAttentionByProjectID(projectID)
	}

	writeJSON(writer, http.StatusOK, pendingResponse{Count: len(pending), Annotations: pending})
}

func (s *Service) handleRequestAction(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.PathValue("id")
	session, ok := s.store.GetSessionWithAnnotations(sessionID)
	if !ok {
		writeError(writer, http.StatusNotFound, "Session not found")
		return
	}

	var input actionRequestInput
	if err := decodeBody(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(input.Output) == "" {
		writeError(writer, http.StatusBadRequest, "output is required")
		return
	}

	action := ActionRequest{
		SessionID:   sessionID,
		Annotations: session.Annotations,
		Output:      input.Output,
		RequestedAt: nowISO(),
	}
	s.store.EmitActionRequested(sessionID, action)

	agent := int(atomic.LoadInt64(&s.agentListeners))
	response := actionResponse{
		Success:         true,
		AnnotationCount: len(session.Annotations),
		Delivered: deliveredInfo{
			SSEListeners: agent,
			Webhooks:     0,
			Total:        agent,
		},
	}
	writeJSON(writer, http.StatusOK, response)
}

func (s *Service) handleSessionEvents(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.PathValue("id")
	_, ok := s.store.GetSession(sessionID)
	if !ok {
		writeError(writer, http.StatusNotFound, "Session not found")
		return
	}

	isAgent := request.URL.Query().Get("agent") == "true"
	if !startSSE(writer) {
		writeError(writer, http.StatusInternalServerError, "streaming not supported")
		return
	}

	clientType := "frontend"
	atomic.AddInt64(&s.activeListeners, 1)
	defer atomic.AddInt64(&s.activeListeners, -1)
	if isAgent {
		clientType = "agent"
		atomic.AddInt64(&s.agentListeners, 1)
		defer atomic.AddInt64(&s.agentListeners, -1)
	}

	s.log.Info("sse connected", "sessionId", sessionID, "client", clientType)
	defer s.log.Info("sse disconnected", "sessionId", sessionID, "client", clientType)

	lastID := parseLastEventID(request.Header.Get("Last-Event-ID"))
	if lastID > 0 {
		for _, event := range s.store.GetEventsSince(sessionID, lastID) {
			if err := writeSSEEvent(writer, event); err != nil {
				return
			}
		}
	}

	events, unsubscribe := s.store.SubscribeSession(sessionID)
	defer unsubscribe()
	s.streamEvents(request.Context(), writer, events)
}

func (s *Service) handleGlobalEvents(writer http.ResponseWriter, request *http.Request) {
	isAgent := request.URL.Query().Get("agent") == "true"
	domain := strings.TrimSpace(request.URL.Query().Get("domain"))
	projectID := strings.TrimSpace(request.URL.Query().Get("projectId"))

	if !startSSE(writer) {
		writeError(writer, http.StatusInternalServerError, "streaming not supported")
		return
	}

	atomic.AddInt64(&s.activeListeners, 1)
	defer atomic.AddInt64(&s.activeListeners, -1)
	if isAgent {
		atomic.AddInt64(&s.agentListeners, 1)
		defer atomic.AddInt64(&s.agentListeners, -1)
		s.sendInitialSync(writer, domain, projectID)
	}

	events, unsubscribe := s.store.SubscribeAll()
	defer unsubscribe()
	s.streamGlobalEvents(request.Context(), writer, events, domain, projectID)
}

func (s *Service) streamEvents(ctx context.Context, writer http.ResponseWriter, events <-chan Event) {
	keepAlive := time.NewTicker(30 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case event := <-events:
			if err := writeSSEEvent(writer, event); err != nil {
				return
			}
		case <-keepAlive.C:
			if err := writeSSEComment(writer, "ping"); err != nil {
				return
			}
		case <-ctx.Done():
			return
		case <-s.shutdownCtx.Done():
			return
		}
	}
}

func (s *Service) streamGlobalEvents(ctx context.Context, writer http.ResponseWriter, events <-chan Event, domain, projectID string) {
	keepAlive := time.NewTicker(30 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case event := <-events:
			if domain != "" && !s.eventMatchesDomain(event, domain) {
				continue
			}
			if projectID != "" && !s.eventMatchesProjectID(event, projectID) {
				continue
			}
			if err := writeSSEEvent(writer, event); err != nil {
				return
			}
		case <-keepAlive.C:
			if err := writeSSEComment(writer, "ping"); err != nil {
				return
			}
		case <-ctx.Done():
			return
		case <-s.shutdownCtx.Done():
			return
		}
	}
}

func (s *Service) sendInitialSync(writer http.ResponseWriter, domain, projectID string) {
	count := 0
	trimmedProjectID := strings.TrimSpace(projectID)

	for _, session := range s.store.ListSessions() {
		if domain != "" && !sessionMatchesDomain(session, domain) {
			continue
		}
		if trimmedProjectID != "" && session.ProjectID != trimmedProjectID {
			continue
		}

		annotations := s.store.GetAnnotationsNeedingAttention(session.ID)
		for _, annotation := range annotations {
			event := Event{
				Type:      EventAnnotationCreated,
				Timestamp: annotation.CreatedAt,
				SessionID: session.ID,
				Sequence:  0,
				Payload:   annotation,
			}
			if err := writeSSEEvent(writer, event); err != nil {
				return
			}
			count++
		}
	}

	syncPayload := map[string]any{
		"domain":    valueOr(domain, "all"),
		"projectId": valueOr(trimmedProjectID, "all"),
		"count":     count,
		"timestamp": nowISO(),
	}
	_ = writeSSECustomEvent(writer, "sync.complete", syncPayload)
}

func (s *Service) eventMatchesDomain(event Event, domain string) bool {
	session, ok := s.store.GetSession(event.SessionID)
	if !ok {
		return false
	}
	return sessionMatchesDomain(session, domain)
}

func (s *Service) eventMatchesProjectID(event Event, projectID string) bool {
	session, ok := s.store.GetSession(event.SessionID)
	if !ok {
		return false
	}
	return strings.TrimSpace(session.ProjectID) == strings.TrimSpace(projectID)
}

func sessionMatchesDomain(session Session, domain string) bool {
	parsed, err := url.Parse(session.URL)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, domain)
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func startSSE(writer http.ResponseWriter) bool {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return false
	}

	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(": connected\n\n"))
	flusher.Flush()
	return true
}

func parseLastEventID(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	if id < 0 {
		return 0
	}
	return id
}

func decodeBody(request *http.Request, target any) error {
	reader := io.LimitReader(request.Body, requestBodyLimit)
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON")
	}
	return nil
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}

func writeSSEEvent(writer http.ResponseWriter, event Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(writer, "event: %s\n", event.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "id: %d\n", event.Sequence); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "data: %s\n\n", payload); err != nil {
		return err
	}
	if flusher, ok := writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func writeSSECustomEvent(writer http.ResponseWriter, name string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "event: %s\n", name); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "data: %s\n\n", encoded); err != nil {
		return err
	}
	if flusher, ok := writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func writeSSEComment(writer http.ResponseWriter, comment string) error {
	if _, err := fmt.Fprintf(writer, ": %s\n\n", comment); err != nil {
		return err
	}
	if flusher, ok := writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}
