package worker

import (
	"context"
	"encoding/json"
	"fmt"
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
			w.dispatchNotifications(ctx)
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
		w.transitionWithNotification(ctx, operation, domain.StatusFailed, "kubernetes_failed", err.Error(), "操作执行失败："+err.Error())
		w.logger.Error("operation failed", zap.String("operation_id", operation.ID.String()), zap.Error(err))
		return
	}
	w.transitionWithNotification(ctx, operation, domain.StatusSucceeded, "", "", "操作执行成功："+operation.ID.String())
}

func (w *Worker) transitionWithNotification(ctx context.Context, operation domain.Operation, status domain.OperationStatus, code, message, notificationText string) {
	var notification *messaging.Notification
	if w.registry != nil && operation.ConversationID != "" && operation.MessageProvider != domain.MessageProviderWeb {
		value := messaging.Notification{OperationID: operation.ID.String(), Destination: operation.ConversationID, Text: notificationText}
		notification = &value
	}
	if err := w.store.TransitionOperationWithNotification(ctx, operation.ID, domain.StatusRunning, status, code, message, notification, operation.MessageProvider); err != nil {
		w.logger.Warn("transition operation", zap.String("operation_id", operation.ID.String()), zap.Error(err))
	}
}

func (w *Worker) dispatchNotifications(ctx context.Context) {
	if w.registry == nil {
		return
	}
	rows, err := w.store.ClaimOutbox(ctx, 20, 30*time.Second)
	if err != nil {
		return
	}
	for _, row := range rows {
		provider, ok := w.registry.Provider(domain.MessageProvider(row.Provider))
		if !ok {
			_ = w.store.MarkOutboxFailed(ctx, row.ID, fmt.Errorf("message provider %s is unavailable", row.Provider))
			continue
		}
		var notification messaging.Notification
		if err := json.Unmarshal(row.Payload, &notification); err != nil {
			_ = w.store.MarkOutboxFailed(ctx, row.ID, err)
			continue
		}
		if err := provider.Send(ctx, notification); err != nil {
			_ = w.store.MarkOutboxFailed(ctx, row.ID, err)
			continue
		}
		_ = w.store.MarkOutboxSent(ctx, row.ID)
	}
}
