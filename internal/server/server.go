package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/config"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
	"github.com/jo-hoe/calendar-assistent/internal/processor"
)

var allowedMIMETypes = map[llm.MIMEType]struct{}{
	llm.MIMEType("image/png"):       {},
	llm.MIMEType("image/jpeg"):      {},
	llm.MIMEType("image/gif"):       {},
	llm.MIMEType("image/webp"):      {},
	llm.MIMEType("application/pdf"): {},
	llm.MIMEType("text/plain"):      {},
	llm.MIMEType("text/html"):       {},
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
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("failed to parse multipart form: %w", err).Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "missing or invalid 'file' field")
		return
	}
	defer func() { _ = file.Close() }()

	mimeType := llm.MIMEType(header.Header.Get("Content-Type"))
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = detectMIMEType(header.Filename)
	}

	if _, ok := allowedMIMETypes[mimeType]; !ok {
		s.writeError(w, http.StatusUnsupportedMediaType, fmt.Sprintf("unsupported file type %q", mimeType))
		return
	}

	result, err := s.processor.ProcessArtifact(r.Context(), file, mimeType)
	if err != nil {
		s.handleProcessingError(w, err)
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

	result, err := s.processor.ProcessArtifact(r.Context(), strings.NewReader(req.Text), llm.MIMEType("text/plain"))
	if err != nil {
		s.handleProcessingError(w, err)
		return
	}

	s.writeJSON(w, http.StatusCreated, result)
}

// handleProcessingError dispatches processing errors to the appropriate HTTP response.
// Extraction failures return 422 Unprocessable Entity; all other errors return 500.
func (s *Server) handleProcessingError(w http.ResponseWriter, err error) {
	if errors.Is(err, processor.ErrCannotExtract) {
		http.Error(w, `{"error":"could not extract event from input"}`, http.StatusUnprocessableEntity)
		return
	}
	s.logger.Error("processing event", "error", err)
	http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
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

func detectMIMEType(filename string) llm.MIMEType {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return llm.MIMEType("image/png")
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return llm.MIMEType("image/jpeg")
	case strings.HasSuffix(lower, ".gif"):
		return llm.MIMEType("image/gif")
	case strings.HasSuffix(lower, ".webp"):
		return llm.MIMEType("image/webp")
	case strings.HasSuffix(lower, ".pdf"):
		return llm.MIMEType("application/pdf")
	case strings.HasSuffix(lower, ".txt"):
		return llm.MIMEType("text/plain")
	case strings.HasSuffix(lower, ".html"), strings.HasSuffix(lower, ".htm"):
		return llm.MIMEType("text/html")
	default:
		return llm.MIMEType("application/octet-stream")
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
