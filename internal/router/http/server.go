package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/benjitaylor/agentation/cli/internal/router/config"
	"github.com/benjitaylor/agentation/cli/internal/router/model"
	routerpkg "github.com/benjitaylor/agentation/cli/internal/router/router"
	"github.com/benjitaylor/agentation/cli/internal/router/store"
)

type Server struct {
	config    config.Config
	logger    *slog.Logger
	registry  *store.Registry
	forwarder *routerpkg.Forwarder
}

func NewServer(cfg config.Config, logger *slog.Logger, registry *store.Registry, forwarder *routerpkg.Forwarder) *http.Server {
	service := &Server{
		config:    cfg,
		logger:    logger,
		registry:  registry,
		forwarder: forwarder,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", service.withCORS(service.handleHealth))
	mux.HandleFunc("/sessions", service.withCORS(service.handleSessions))
	mux.HandleFunc("/register", service.withCORS(service.handleRegister))
	mux.HandleFunc("/unregister", service.withCORS(service.handleUnregister))
	mux.HandleFunc("/ping", service.withCORS(service.handlePing))
	mux.HandleFunc("/open", service.withCORS(service.handleOpen))

	return &http.Server{
		Addr:              cfg.Address,
		Handler:           mux,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    8 * 1024,
	}
}

func (s *Server) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Agentation-Token")
		writer.Header().Set("Content-Type", "application/json")

		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}

		next(writer, request)
	}
}

func (s *Server) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSessions(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessions := s.registry.List()
	writeJSON(writer, http.StatusOK, map[string]any{
		"sessions": sessions,
	})
}

func (s *Server) handleRegister(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.isAuthorized(request) {
		s.logger.Warn("unauthorized register request")
		writeError(writer, http.StatusUnauthorized, "unauthorized")
		return
	}

	var payload model.RegisterRequest
	if error := decodeJSONBody(request, s.config.RequestBodyLimit, &payload); error != nil {
		writeError(writer, http.StatusBadRequest, error.Error())
		return
	}

	if strings.TrimSpace(payload.SessionID) == "" {
		writeError(writer, http.StatusBadRequest, "sessionId is required")
		return
	}
	if strings.TrimSpace(payload.Endpoint) == "" {
		writeError(writer, http.StatusBadRequest, "endpoint is required")
		return
	}

	session, outcome := s.registry.RegisterWithOutcome(payload)
	switch outcome {
	case store.RegisterOutcomeNew:
		s.logger.Info(
			"session connected",
			"sessionId", session.SessionID,
			"projectId", session.ProjectID,
			"repoId", session.RepoID,
			"endpoint", session.Endpoint,
			"root", session.Root,
		)
	case store.RegisterOutcomeUpdated:
		s.logger.Info(
			"session updated",
			"sessionId", session.SessionID,
			"projectId", session.ProjectID,
			"repoId", session.RepoID,
			"endpoint", session.Endpoint,
			"root", session.Root,
		)
	}

	writeJSON(writer, http.StatusOK, map[string]any{
		"session": session,
	})
}

func (s *Server) handleUnregister(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.isAuthorized(request) {
		s.logger.Warn("unauthorized unregister request")
		writeError(writer, http.StatusUnauthorized, "unauthorized")
		return
	}

	var payload model.UnregisterRequest
	if error := decodeJSONBody(request, s.config.RequestBodyLimit, &payload); error != nil {
		writeError(writer, http.StatusBadRequest, error.Error())
		return
	}

	if strings.TrimSpace(payload.SessionID) == "" {
		writeError(writer, http.StatusBadRequest, "sessionId is required")
		return
	}

	s.registry.Unregister(payload.SessionID)
	s.logger.Info("session unregistered", "sessionId", payload.SessionID)
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePing(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	pingRequest, error := parsePingRequest(request, s.config.RequestBodyLimit)
	if error != nil {
		writeError(writer, http.StatusBadRequest, error.Error())
		return
	}

	session, resolveError := s.registry.Resolve(strings.TrimSpace(pingRequest.ProjectID), "", strings.TrimSpace(pingRequest.Origin))
	if resolveError != nil {
		s.respondResolveError(writer, resolveError)
		return
	}

	if error := s.forwarder.ForwardPing(request.Context(), session); error != nil {
		s.logger.Debug("forward ping failed", "sessionId", session.SessionID, "error", error)
		writeError(writer, http.StatusBadGateway, "could not reach target session")
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOpen(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	openRequest, error := parseOpenRequest(request, s.config.RequestBodyLimit)
	if error != nil {
		writeError(writer, http.StatusBadRequest, error.Error())
		return
	}

	normalizedPath, error := normalizeOpenPath(openRequest.Path, s.config.AllowAbsolutePaths)
	if error != nil {
		writeError(writer, http.StatusBadRequest, error.Error())
		return
	}
	openRequest.Path = normalizedPath

	session, resolveError := s.registry.Resolve(strings.TrimSpace(openRequest.ProjectID), normalizedPath, strings.TrimSpace(openRequest.Origin))
	if resolveError != nil {
		s.logger.Warn(
			"open request session resolution failed",
			"projectId", openRequest.ProjectID,
			"origin", openRequest.Origin,
			"path", openRequest.Path,
			"error", resolveError,
		)
		s.respondResolveError(writer, resolveError)
		return
	}

	if error := validatePathForSession(openRequest.Path, session, s.config.AllowAbsolutePaths, s.config.EnforceRootBounds); error != nil {
		s.logger.Warn(
			"open request path rejected",
			"sessionId", session.SessionID,
			"path", openRequest.Path,
			"error", error,
		)
		writeError(writer, http.StatusBadRequest, error.Error())
		return
	}

	if error := s.forwarder.ForwardOpen(request.Context(), session, openRequest); error != nil {
		s.logger.Warn("forward open failed", "sessionId", session.SessionID, "error", error)
		writeError(writer, http.StatusBadGateway, "could not reach target session")
		return
	}

	s.logger.Info(
		"open request routed",
		"sessionId", session.SessionID,
		"projectId", session.ProjectID,
		"origin", openRequest.Origin,
		"path", openRequest.Path,
		"line", openRequest.Line,
		"column", openRequest.Column,
	)

	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) respondResolveError(writer http.ResponseWriter, resolveError error) {
	if errors.Is(resolveError, store.ErrNoMatchingSession) {
		writeError(writer, http.StatusNotFound, "no matching Neovim session")
		return
	}
	if errors.Is(resolveError, store.ErrAmbiguousSession) {
		writeJSON(writer, http.StatusConflict, map[string]any{
			"error":    "ambiguous session",
			"sessions": s.registry.List(),
		})
		return
	}
	writeError(writer, http.StatusInternalServerError, "session resolution failed")
}

func (s *Server) isAuthorized(request *http.Request) bool {
	if s.config.AuthToken == "" {
		return true
	}

	providedToken := strings.TrimSpace(request.Header.Get("X-Agentation-Token"))
	if providedToken == "" {
		authorization := strings.TrimSpace(request.Header.Get("Authorization"))
		providedToken = strings.TrimPrefix(authorization, "Bearer ")
		providedToken = strings.TrimSpace(providedToken)
	}
	return providedToken == s.config.AuthToken
}

func parsePingRequest(request *http.Request, bodyLimit int64) (model.PingRequest, error) {
	if request.Method == http.MethodGet {
		return model.PingRequest{
			ProjectID: request.URL.Query().Get("projectId"),
			Origin:    request.URL.Query().Get("origin"),
		}, nil
	}

	var payload model.PingRequest
	if error := decodeJSONBody(request, bodyLimit, &payload); error != nil {
		return model.PingRequest{}, error
	}
	return payload, nil
}

func parseOpenRequest(request *http.Request, bodyLimit int64) (model.OpenRequest, error) {
	if request.Method == http.MethodGet {
		line := parseIntOrDefault(request.URL.Query().Get("line"), 1)
		column := parseIntOrDefault(request.URL.Query().Get("column"), 1)
		return model.OpenRequest{
			ProjectID: request.URL.Query().Get("projectId"),
			Path:      request.URL.Query().Get("path"),
			Line:      line,
			Column:    column,
			Origin:    request.URL.Query().Get("origin"),
		}, nil
	}

	var payload model.OpenRequest
	if error := decodeJSONBody(request, bodyLimit, &payload); error != nil {
		return model.OpenRequest{}, error
	}
	if payload.Line <= 0 {
		payload.Line = 1
	}
	if payload.Column <= 0 {
		payload.Column = 1
	}
	return payload, nil
}

func normalizeOpenPath(pathValue string, allowAbsolutePaths bool) (string, error) {
	trimmed := strings.TrimSpace(pathValue)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.ContainsRune(trimmed, '\x00') {
		return "", fmt.Errorf("path contains invalid null byte")
	}

	cleaned := filepath.Clean(trimmed)
	if cleaned == "." {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(cleaned) {
		if !allowAbsolutePaths {
			return "", fmt.Errorf("absolute paths are not allowed")
		}
		return cleaned, nil
	}
	if strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("path traversal is not allowed")
	}
	return cleaned, nil
}

func validatePathForSession(pathValue string, session model.Session, allowAbsolutePaths bool, enforceRootBounds bool) error {
	if !filepath.IsAbs(pathValue) {
		return nil
	}
	if !allowAbsolutePaths {
		return fmt.Errorf("absolute paths are not allowed")
	}
	if !enforceRootBounds {
		return nil
	}
	if strings.TrimSpace(session.Root) == "" {
		return fmt.Errorf("target session has no root configured")
	}
	cleanPath := filepath.Clean(pathValue)
	cleanRoot := filepath.Clean(session.Root)
	if cleanPath == cleanRoot {
		return nil
	}
	if strings.HasPrefix(filepath.ToSlash(cleanPath), filepath.ToSlash(cleanRoot)+"/") {
		return nil
	}
	return fmt.Errorf("path is outside session root")
}

func decodeJSONBody(request *http.Request, bodyLimit int64, target any) error {
	if request.Body == nil {
		return fmt.Errorf("request body is required")
	}
	defer request.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(request.Body, bodyLimit))
	decoder.DisallowUnknownFields()

	if error := decoder.Decode(target); error != nil {
		return fmt.Errorf("invalid JSON body: %w", error)
	}

	if decoder.More() {
		return fmt.Errorf("invalid JSON body: trailing content")
	}
	return nil
}

func parseIntOrDefault(value string, fallback int) int {
	parsed := fallback
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
	if parsed <= 0 {
		return fallback
	}
	return parsed
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}
