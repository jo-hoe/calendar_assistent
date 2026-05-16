package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/config"
	"github.com/jo-hoe/calendar-assistent/internal/processor"
)

var allowedMIMETypes = map[string]bool{
	"image/png":       true,
	"image/jpeg":      true,
	"image/gif":       true,
	"image/webp":      true,
	"application/pdf": true,
	"text/plain":      true,
	"text/html":       true,
}

type Server struct {
	httpServer *http.Server
	processor  *processor.Processor
	apiKey     string
	maxUpload  int64
	logger     *slog.Logger
}

func New(cfg config.ServerConfig, proc *processor.Processor, logger *slog.Logger) *Server {
	s := &Server{
		processor: proc,
		apiKey:    cfg.APIKey,
		maxUpload: int64(cfg.MaxUpload),
		logger:    logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /v1/events/artifact", s.withAuth(s.handleArtifact))
	mux.HandleFunc("POST /v1/events/text", s.withAuth(s.handleText))

	s.httpServer = &http.Server{
		Addr:         cfg.Address,
		Handler:      s.withLogging(s.withRecovery(mux)),
		ReadTimeout:  cfg.ReadTimeout.Duration,
		WriteTimeout: cfg.WriteTimeout.Duration,
		IdleTimeout:  cfg.IdleTimeout.Duration,
	}

	return s
}

func (s *Server) ListenAndServe() error {
	s.logger.Info("starting server", "address", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, `{"status":"ok"}`)
}

func (s *Server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUpload)

	if err := r.ParseMultipartForm(s.maxUpload); err != nil {
		s.writeError(w, http.StatusBadRequest, "failed to parse multipart form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "missing or invalid 'file' field")
		return
	}
	defer func() { _ = file.Close() }()

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = detectMIMEType(header.Filename)
	}

	if !allowedMIMETypes[mimeType] {
		s.writeError(w, http.StatusUnsupportedMediaType, fmt.Sprintf("unsupported file type %q", mimeType))
		return
	}

	result, err := s.processor.ProcessArtifact(r.Context(), file, mimeType)
	if err != nil {
		if strings.Contains(err.Error(), "could not extract") {
			s.writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.logger.Error("processing artifact", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error processing artifact")
		return
	}

	s.writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleText(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUpload)

	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	if strings.TrimSpace(req.Text) == "" {
		s.writeError(w, http.StatusBadRequest, "text field is required and must not be empty")
		return
	}

	result, err := s.processor.ProcessArtifact(r.Context(), strings.NewReader(req.Text), "text/plain")
	if err != nil {
		if strings.Contains(err.Error(), "could not extract") {
			s.writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.logger.Error("processing text", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error processing text")
		return
	}

	s.writeJSON(w, http.StatusCreated, result)
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey != "" {
			key := r.Header.Get("X-API-Key")
			if key != s.apiKey {
				s.writeError(w, http.StatusUnauthorized, "invalid or missing API key")
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("panic recovered", "panic", rec)
				s.writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration", time.Since(start),
			"remote", r.RemoteAddr,
		)
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg})
}

func detectMIMEType(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".txt"):
		return "text/plain"
	case strings.HasSuffix(lower, ".html"), strings.HasSuffix(lower, ".htm"):
		return "text/html"
	default:
		return "application/octet-stream"
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
