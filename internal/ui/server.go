package ui

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"discord-osint/internal/export"
	"discord-osint/internal/store"

	"go.uber.org/zap"
)

//go:embed assets/index.html
var indexHTML []byte

// ServerOptions configures the embedded dashboard HTTP server.
type ServerOptions struct {
	Addr   string
	Store  *store.Store
	Logger *zap.Logger
}

// Server serves the web dashboard and REST endpoints.
type Server struct {
	server *http.Server
	store  *store.Store
	logger *zap.Logger
	addr   string
}

// NewServer initializes a new web dashboard server.
func NewServer(opts ServerOptions) *Server {
	addr := opts.Addr
	if addr == "" {
		addr = "127.0.0.1:3000"
	}
	log := opts.Logger
	if log == nil {
		log = zap.NewNop()
	}

	s := &Server{
		store:  opts.Store,
		logger: log,
		addr:   addr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/runs", s.handleListRuns)
	mux.HandleFunc("/api/runs/", s.handleRunDetail)
	mux.HandleFunc("/api/run/", s.handleRunDetail)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	return s
}

// Start runs the HTTP server. It blocks until closed or error.
func (s *Server) Start() error {
	s.logger.Info("Starting Discord OSINT web dashboard", zap.String("addr", s.addr))
	err := s.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// StartListener starts listening on the given net.Listener (useful for testing dynamic ports).
func (s *Server) StartListener(ln net.Listener) error {
	err := s.server.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown gracefully shuts down the dashboard server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexHTML)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.store.ListRuns(r.Context())
	if err != nil {
		s.logger.Error("Failed to list runs", zap.Error(err))
		http.Error(w, "Failed to retrieve runs", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(runs)
}

// RunDetailPayload aggregates target, scan history, observations, and messages for a run.
type RunDetailPayload struct {
	Run          *store.RunRecord         `json:"run"`
	Scans        []store.GuildScanRecord  `json:"scans"`
	Observations []store.ObservationRecord `json:"observations"`
	Messages     []store.MessageRecord    `json:"messages"`
}

func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	// Paths handled: /api/runs/{id} or /api/runs/{id}/report.html
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/api/runs/")
	path = strings.TrimPrefix(path, "/api/run/")

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Run ID required", http.StatusBadRequest)
		return
	}

	runID := parts[0]
	ctx := r.Context()

	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			http.Error(w, "Run not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to load run", http.StatusInternalServerError)
		return
	}

	scans, err := s.store.GetRunGuildScans(ctx, runID)
	if err != nil {
		http.Error(w, "Failed to load scans", http.StatusInternalServerError)
		return
	}

	obs, err := s.store.GetRunObservations(ctx, runID)
	if err != nil {
		http.Error(w, "Failed to load observations", http.StatusInternalServerError)
		return
	}

	msgs, err := s.store.GetRunMessages(ctx, runID)
	if err != nil {
		http.Error(w, "Failed to load messages", http.StatusInternalServerError)
		return
	}

	// If requesting HTML report: /api/runs/{id}/report.html
	if len(parts) > 1 && parts[1] == "report.html" {
		htmlReport := export.GenerateHTMLReport(run, scans, obs, msgs)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(htmlReport))
		return
	}

	payload := RunDetailPayload{
		Run:          run,
		Scans:        scans,
		Observations: obs,
		Messages:     msgs,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
