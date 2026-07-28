package worker

import (
	"context"
	"time"

	"chatops-deploy/internal/adapter/kubernetes"
	"chatops-deploy/internal/application"
	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/messaging"
	"chatops-deploy/internal/store/postgres"
	"go.uber.org/zap"
)

type Worker struct {
	id       string
	interval time.Duration
	store    *postgres.Store
	k8s      *kubernetes.Manager
	logger   *zap.Logger
	registry *messaging.Registry
}

func New(id string, interval time.Duration, store *postgres.Store, k8s *kubernetes.Manager, registry *messaging.Registry, logger *zap.Logger) *Worker {
	return &Worker{id: id, interval: interval, store: store, k8s: k8s, registry: registry, logger: logger}
}
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOne(ctx)
		}
	}
}
func (w *Worker) runOne(ctx context.Context) {
	operation, err := w.store.ClaimOperation(ctx, w.id)
	if err != nil {
		return
	}
	env, err := w.store.GetEnvironment(ctx, operation.EnvironmentID)
	if err == nil {
		timeout := application.OperationTimeout(env)
		if operation.Kind == domain.OperationDeploy {
			_, err = w.k8s.Deploy(ctx, env, operation.Image, timeout)
		} else {
			_, err = w.k8s.Rollback(ctx, env, operation.Revision, timeout)
		}
	}
	if err != nil {
		_ = w.store.TransitionOperation(ctx, operation.ID, domain.StatusRunning, domain.StatusFailed, "kubernetes_failed", err.Error())
		w.notify(ctx, operation, "操作执行失败："+err.Error())
		w.logger.Error("operation failed", zap.String("operation_id", operation.ID.String()), zap.Error(err))
		return
	}
	_ = w.store.TransitionOperation(ctx, operation.ID, domain.StatusRunning, domain.StatusSucceeded, "", "")
	w.notify(ctx, operation, "操作执行成功："+operation.ID.String())
}

func (w *Worker) notify(ctx context.Context, operation domain.Operation, text string) {
	if w.registry == nil || operation.ConversationID == "" || operation.MessageProvider == domain.MessageProviderWeb {
		return
	}
	provider, ok := w.registry.Provider(operation.MessageProvider)
	if !ok {
		w.logger.Warn("operation message provider unavailable", zap.String("provider", string(operation.MessageProvider)), zap.String("operation_id", operation.ID.String()))
		return
	}
	if err := provider.Send(ctx, messaging.Notification{OperationID: operation.ID.String(), Destination: operation.ConversationID, Text: text}); err != nil {
		w.logger.Warn("send operation notification", zap.String("provider", string(operation.MessageProvider)), zap.String("operation_id", operation.ID.String()), zap.Error(err))
	}
}
