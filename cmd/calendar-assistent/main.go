package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/calendar"
	"github.com/jo-hoe/calendar-assistent/internal/config"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
	"github.com/jo-hoe/calendar-assistent/internal/processor"
	"github.com/jo-hoe/calendar-assistent/internal/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load("")
	if err != nil {
		logger.Error("loading config", "error", err)
		os.Exit(1)
	}

	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLogLevel(cfg.Server.LogLevel)}))

	llmClient, err := llm.NewClient(cfg.LLM)
	if err != nil {
		logger.Error("creating LLM client", "error", err)
		os.Exit(1)
	}

	calProvider, err := calendar.NewProvider(cfg.Calendar, logger)
	if err != nil {
		logger.Error("creating calendar provider", "error", err)
		os.Exit(1)
	}

	proc := processor.New(llmClient, calProvider)
	srv := server.New(cfg.Server, proc, logger)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info("shutting down gracefully")
	case err := <-errCh:
		logger.Error("server error", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Warn("shutdown error", "error", err)
	}
}

func parseLogLevel(s config.LogLevel) slog.Level {
	switch s {
	case config.LogLevelDebug:
		return slog.LevelDebug
	case config.LogLevelWarn, config.LogLevelWarning:
		return slog.LevelWarn
	case config.LogLevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
