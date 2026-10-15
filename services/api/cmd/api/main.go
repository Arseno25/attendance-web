package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"attendance.local/api/internal/attendance"
	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/masterdata"
	"attendance.local/api/internal/platform/aiclient"
	"attendance.local/api/internal/platform/config"
	"attendance.local/api/internal/platform/httpserver"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("api stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	defer pool.Close()
	startup, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	schoolID, err := auth.ResolveSchoolID(startup, pool)
	cancelStartup()
	if err != nil {
		return err
	}
	authService, err := auth.NewWithLocalOrigins(pool, schoolID, cfg.WebOrigin, cfg.AllowLocalWebPortRange, cfg.TrustedProxies...)
	if err != nil {
		return errors.New("authentication initialization failed")
	}
	var aiClient *aiclient.Client
	if cfg.AIBaseURL != "" {
		aiClient, err = aiclient.New(cfg.AIBaseURL, cfg.AIServiceToken)
		if err != nil {
			return err
		}
	}
	masterService := masterdata.New(pool, schoolID, authService)
	masterService.SetAI(aiClient)
	attendanceService := attendance.New(pool, schoolID, authService)
	attendanceService.SetAI(aiClient)
	go attendanceService.Run(ctx)
	server := &http.Server{
		Addr: cfg.Address,
		Handler: httpserver.NewHandlerWithLocalOrigins(pool.Ping, cfg.WebOrigin, cfg.AllowLocalWebPortRange, func(m *http.ServeMux) {
			authService.Register(m)
			masterService.Register(m)
			attendanceService.Register(m)
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	slog.Info("api listening", "address", cfg.Address)
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}
