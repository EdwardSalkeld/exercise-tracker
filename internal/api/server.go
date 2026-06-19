package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/EdwardSalkeld/exercise-tracker/internal/model"
)

var errNotFound = errors.New("not found")
var errInvalidInput = errors.New("invalid input")

func ErrNotFound() error {
	return errNotFound
}

func ErrInvalidInput() error {
	return errInvalidInput
}

type QueryService interface {
	HealthCheck(ctx context.Context) error
	ListSessions(ctx context.Context, limit int) ([]model.SessionSummary, error)
	GetSession(ctx context.Context, id int64) (model.SessionDetail, error)
	UpdateSession(ctx context.Context, id int64, input model.SessionCreate) (model.SessionDetail, error)
	DeleteSession(ctx context.Context, id int64) error
	ExerciseHistory(ctx context.Context, baseName string, limit int) ([]model.ExerciseHistoryItem, error)
	ListRuns(ctx context.Context, limit int) ([]model.RunSummary, error)
	GetRun(ctx context.Context, id int64) (model.RunDetail, error)
	UpdateRun(ctx context.Context, id int64, input model.RunCreate) (model.RunDetail, error)
	DeleteRun(ctx context.Context, id int64) error
	CreateSession(ctx context.Context, input model.SessionCreate) (model.SessionDetail, error)
	CreateRun(ctx context.Context, input model.RunCreate) (model.RunDetail, error)
}

type Server struct {
	store QueryService
}

func NewServer(store QueryService) *Server {
	return &Server{store: store}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/sessions", s.handleSessions)
	mux.HandleFunc("/v1/sessions/", s.handleSessionByID)
	mux.HandleFunc("/v1/exercises/", s.handleExerciseRoutes)
	mux.HandleFunc("/v1/runs", s.handleRuns)
	mux.HandleFunc("/v1/runs/", s.handleRunByID)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	if err := s.store.HealthCheck(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, err := parseLimit(r, 20, 100)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sessions, err := s.store.ListSessions(r.Context(), limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list sessions failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": sessions,
			"limit": limit,
		})
	case http.MethodPost:
		var input model.SessionCreate
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		session, err := s.store.CreateSession(r.Context(), input)
		if err != nil {
			if errors.Is(err, errInvalidInput) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "create session failed")
			return
		}
		writeJSON(w, http.StatusCreated, session)
	default:
		writeMethodNotAllowed(w)
	}
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDPath(r.URL.Path, "/v1/sessions/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch r.Method {
	case http.MethodGet:
		session, err := s.store.GetSession(r.Context(), id)
		if err != nil {
			if errors.Is(err, errNotFound) {
				writeError(w, http.StatusNotFound, "session not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "get session failed")
			return
		}
		writeJSON(w, http.StatusOK, session)
	case http.MethodPut:
		var input model.SessionCreate
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		session, err := s.store.UpdateSession(r.Context(), id, input)
		if err != nil {
			if errors.Is(err, errNotFound) {
				writeError(w, http.StatusNotFound, "session not found")
				return
			}
			if errors.Is(err, errInvalidInput) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "update session failed")
			return
		}
		writeJSON(w, http.StatusOK, session)
	case http.MethodDelete:
		err := s.store.DeleteSession(r.Context(), id)
		if err != nil {
			if errors.Is(err, errNotFound) {
				writeError(w, http.StatusNotFound, "session not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "delete session failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w)
	}
}

func (s *Server) handleExerciseRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/exercises/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "history" || parts[0] == "" {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}
	limit, err := parseLimit(r, 50, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := s.store.ExerciseHistory(r.Context(), parts[0], limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "exercise history failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"exercise": parts[0],
		"items":    items,
		"limit":    limit,
	})
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, err := parseLimit(r, 20, 100)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		runs, err := s.store.ListRuns(r.Context(), limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list runs failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": runs,
			"limit": limit,
		})
	case http.MethodPost:
		var input model.RunCreate
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		run, err := s.store.CreateRun(r.Context(), input)
		if err != nil {
			if errors.Is(err, errInvalidInput) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "create run failed")
			return
		}
		writeJSON(w, http.StatusCreated, run)
	default:
		writeMethodNotAllowed(w)
	}
}

func (s *Server) handleRunByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDPath(r.URL.Path, "/v1/runs/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch r.Method {
	case http.MethodGet:
		run, err := s.store.GetRun(r.Context(), id)
		if err != nil {
			if errors.Is(err, errNotFound) {
				writeError(w, http.StatusNotFound, "run not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "get run failed")
			return
		}
		writeJSON(w, http.StatusOK, run)
	case http.MethodPut:
		var input model.RunCreate
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		run, err := s.store.UpdateRun(r.Context(), id, input)
		if err != nil {
			if errors.Is(err, errNotFound) {
				writeError(w, http.StatusNotFound, "run not found")
				return
			}
			if errors.Is(err, errInvalidInput) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "update run failed")
			return
		}
		writeJSON(w, http.StatusOK, run)
	case http.MethodDelete:
		err := s.store.DeleteRun(r.Context(), id)
		if err != nil {
			if errors.Is(err, errNotFound) {
				writeError(w, http.StatusNotFound, "run not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "delete run failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w)
	}
}

func parseLimit(r *http.Request, fallback int, max int) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, errors.New("limit must be a positive integer")
	}
	if value > max {
		return max, nil
	}
	return value, nil
}

func parseIDPath(path string, prefix string) (int64, error) {
	raw := strings.TrimPrefix(path, prefix)
	raw = strings.Trim(raw, "/")
	if raw == "" || strings.Contains(raw, "/") {
		return 0, errors.New("invalid id path")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, errors.New("id must be a positive integer")
	}
	return value, nil
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid JSON body")
	}
	if decoder.More() {
		return errors.New("invalid JSON body")
	}
	return nil
}
