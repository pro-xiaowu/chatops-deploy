package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"chatops-deploy/internal/adapter/dingtalk"
	"chatops-deploy/internal/adapter/feishu"
	kubeadapter "chatops-deploy/internal/adapter/kubernetes"
	messagingweb "chatops-deploy/internal/adapter/messaging"
	"chatops-deploy/internal/adapter/wecom"
	"chatops-deploy/internal/application"
	"chatops-deploy/internal/auth"
	"chatops-deploy/internal/config"
	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/logging"
	"chatops-deploy/internal/messaging"
	"chatops-deploy/internal/security"
	storepostgres "chatops-deploy/internal/store/postgres"
	transporthttp "chatops-deploy/internal/transport/http"
	transporthandler "chatops-deploy/internal/transport/http/handler"
	"chatops-deploy/internal/worker"
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
	openCtx, cancelOpen := context.WithTimeout(ctx, 15*time.Second)
	defer cancelOpen()
	db, sqlDB, err := storepostgres.Open(openCtx, cfg.Database.URL, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.ConnMaxLifetime)
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err = storepostgres.Migrate(openCtx, db); err != nil {
		return err
	}
	box, err := security.NewSecretBox(cfg.Security.KubeconfigMasterKey)
	if err != nil {
		return err
	}
	store := storepostgres.New(db)
	tokens := auth.New(db)
	if err = tokens.EnsureBootstrap(openCtx, cfg.Security.BootstrapAdminToken); err != nil {
		return err
	}
	app := application.New(store, box)
	feishuClient := feishu.NewClient(cfg.Feishu.AppID, cfg.Feishu.AppSecret, cfg.Feishu.APIBaseURL)
	providers := []messaging.Provider{
		messagingweb.New(),
		feishu.NewProvider(cfg.Feishu),
		wecom.NewProvider(cfg.WeCom),
		dingtalk.NewProvider(cfg.DingTalk),
	}
	registry := messaging.NewRegistry(store, providers)
	configuredProvider := domain.MessageProvider(cfg.MessageProvider)
	provider, ok := registry.Provider(configuredProvider)
	if !ok || !provider.Capabilities(openCtx).Configured {
		return fmt.Errorf("configured message provider %s is unavailable", configuredProvider)
	}
	if err = store.InitializeActiveMessageProvider(openCtx, configuredProvider); err != nil {
		return err
	}
	if cfg.DevAuthEnabled {
		if _, err = store.EnsureDevelopmentAdmin(openCtx); err != nil {
			return err
		}
	}
	api := transporthandler.NewAPIWithOptions(app, store, tokens, feishuClient, cfg.Feishu.VerificationToken, cfg.Feishu.EncryptKey, cfg.Security.PublicBaseURL, cfg.Security.CookieSecure, registry, cfg.RuntimeEnvironment, cfg.DevAuthEnabled)
	kube := kubeadapter.NewManager(store, box)
	router := transporthttp.NewRouter(transporthttp.Dependencies{
		Readiness: store.Ready, API: api, Tokens: tokens,
	})
	if cfg.Mode == "all" || cfg.Mode == "worker" {
		go worker.New(cfg.Worker.ID, cfg.Worker.PollInterval, store, kube, registry, logger).Run(ctx)
	}
	if cfg.Mode == "worker" {
		<-ctx.Done()
		return nil
	}
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
