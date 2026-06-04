package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Nextasy01/chtiwt/internal/auth"
	"github.com/Nextasy01/chtiwt/internal/chat"
	"github.com/Nextasy01/chtiwt/internal/config"
	"github.com/Nextasy01/chtiwt/internal/store"
	"github.com/Nextasy01/chtiwt/internal/stream"
	"github.com/Nextasy01/chtiwt/internal/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := store.Migrate(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	slog.Info("db migrated")

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	slog.Info("db connected")

	authSvc := auth.NewService(db.Pool, cfg.SessionTTL)
	if err := authSvc.SweepExpiredSessions(ctx); err != nil {
		slog.Warn("session sweep on boot failed", "err", err)
	}

	streamSvc := stream.NewService(stream.Options{
		Pool:       db.Pool,
		RTMPAddr:   cfg.RTMPAddr,
		StateDir:   cfg.StateDir,
		FFmpegPath: cfg.FFmpegPath,
	})
	if err := streamSvc.RecoverOnBoot(ctx); err != nil {
		return fmt.Errorf("stream recover: %w", err)
	}

	chatSvc := chat.NewService(chat.Options{})

	tmpl, err := web.LoadTemplates()
	if err != nil {
		return fmt.Errorf("load templates: %w", err)
	}

	authHandlers := auth.NewHandlers(authSvc, tmpl, cfg.SecureCookies)
	webSrv := web.NewServer(authSvc, streamSvc, chatSvc, cfg.StateDir, tmpl)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(pingCtx); err != nil {
			http.Error(w, "db unhealthy: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	authHandlers.Mount(mux)
	webSrv.Mount(mux)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           requestLogger(authSvc.Middleware(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	httpErr := make(chan error, 1)
	go func() {
		slog.Info("http listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			httpErr <- err
		}
		close(httpErr)
	}()

	rtmpErr := make(chan error, 1)
	go func() {
		if err := streamSvc.ListenAndServeRTMP(ctx); err != nil {
			rtmpErr <- err
		}
		close(rtmpErr)
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err, ok := <-httpErr:
		if ok && err != nil {
			return fmt.Errorf("http server: %w", err)
		}
	case err, ok := <-rtmpErr:
		if ok && err != nil {
			return fmt.Errorf("rtmp server: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	streamSvc.ShutdownLive()
	chatSvc.Shutdown()
	slog.Info("shutdown complete")
	return nil
}

func requestLogger(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h.ServeHTTP(w, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"dur_ms", time.Since(start).Milliseconds(),
		)
	})
}
