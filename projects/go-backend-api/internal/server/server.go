package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type Server struct {
	db        *sql.DB
	logger    *slog.Logger
	startedAt time.Time
	requests  atomic.Uint64
	errors    atomic.Uint64
}

type Task struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
}

type createTaskRequest struct {
	Title string `json:"title"`
}

func New(db *sql.DB, logger *slog.Logger) *Server {
	return &Server{db: db, logger: logger, startedAt: time.Now().UTC()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.ready)
	mux.HandleFunc("GET /metrics", s.metrics)
	mux.HandleFunc("GET /api/v1/tasks", s.listTasks)
	mux.HandleFunc("POST /api/v1/tasks", s.createTask)
	mux.HandleFunc("PATCH /api/v1/tasks/{id}/complete", s.completeTask)

	return s.middleware(mux)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		s.requests.Add(1)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.status >= 500 {
			s.errors.Add(1)
		}
		s.logger.Info("http_request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"service":        "go-backend-api",
		"uptime_seconds": int64(time.Since(s.startedAt).Seconds()),
	})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.db.PingContext(ctx); err != nil {
		s.logger.Error("readiness database ping failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(
		"# HELP app_http_requests_total Total HTTP requests.\n" +
			"# TYPE app_http_requests_total counter\n" +
			"app_http_requests_total " + strconv.FormatUint(s.requests.Load(), 10) + "\n" +
			"# HELP app_http_errors_total Total HTTP 5xx responses.\n" +
			"# TYPE app_http_errors_total counter\n" +
			"app_http_errors_total " + strconv.FormatUint(s.errors.Load(), 10) + "\n",
	))
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT id, title, completed, created_at
		   FROM tasks
		  ORDER BY id DESC
		  LIMIT 100`)
	if err != nil {
		s.internalError(w, err)
		return
	}
	defer rows.Close()

	tasks := make([]Task, 0)
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.Title, &task.Completed, &task.CreatedAt); err != nil {
			s.internalError(w, err)
			return
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var input createTaskRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}

	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" || len(input.Title) > 200 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "title_must_be_1_to_200_characters"})
		return
	}

	var task Task
	err := s.db.QueryRowContext(r.Context(),
		`INSERT INTO tasks (title)
		 VALUES ($1)
		 RETURNING id, title, completed, created_at`,
		input.Title,
	).Scan(&task.ID, &task.Title, &task.Completed, &task.CreatedAt)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (s *Server) completeTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_task_id"})
		return
	}

	var task Task
	err = s.db.QueryRowContext(r.Context(),
		`UPDATE tasks
		    SET completed = TRUE
		  WHERE id = $1
		  RETURNING id, title, completed, created_at`,
		id,
	).Scan(&task.ID, &task.Title, &task.Completed, &task.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task_not_found"})
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	s.logger.Error("request failed", "error", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_server_error"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
