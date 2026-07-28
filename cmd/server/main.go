package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"chatops-deploy/internal/config"
	"chatops-deploy/internal/logging"
	transporthttp "chatops-deploy/internal/transport/http"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}

	logger, err := logging.New(cfg.Log)
	if err != nil {
		log.Fatalf("configure logging: %v", err)
	}
	defer func() { _ = logger.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, logger); err != nil {
		logger.Fatal("server stopped", zap.Error(err))
	}
}

func run(ctx context.Context, cfg config.Config, logger *zap.Logger) error {
	router := transporthttp.NewRouter(transporthttp.Dependencies{
		Readiness: func(context.Context) error { return nil },
	})
	server := &http.Server{
		Addr:    cfg.HTTP.Addr,
		Handler: router,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", zap.String("addr", cfg.HTTP.Addr))
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}

	if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve http: %w", err)
	}
	logger.Info("http server stopped")
	return nil
}
