package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/security"
	"chatops-deploy/internal/store/postgres"
	"github.com/google/uuid"
)

type Service struct {
	store *postgres.Store
	box   *security.SecretBox
}

func New(store *postgres.Store, box *security.SecretBox) *Service {
	return &Service{store: store, box: box}
}
func (s *Service) Store() *postgres.Store { return s.store }

type CreateClusterInput struct{ Name, APIServer, Kubeconfig string }

func (s *Service) CreateCluster(ctx context.Context, in CreateClusterInput) (domain.Cluster, error) {
	id := uuid.New()
	version := int64(1)
	cipher, err := s.box.Seal([]byte(in.Kubeconfig), []byte(fmt.Sprintf("%s:%d", id, version)))
	if err != nil {
		return domain.Cluster{}, err
	}
	return s.store.CreateCluster(ctx, domain.Cluster{ID: id, Name: strings.TrimSpace(in.Name), APIServer: strings.TrimSpace(in.APIServer), EncryptedKubeconfig: cipher, CredentialVersion: version, Enabled: true})
}
func (s *Service) CreateApplication(ctx context.Context, name, description string) (domain.Application, error) {
	return s.store.CreateApplication(ctx, domain.Application{Name: strings.ToLower(strings.TrimSpace(name)), Description: description, Enabled: true})
}
func (s *Service) CreateEnvironment(ctx context.Context, e domain.AppEnvironment) (domain.AppEnvironment, error) {
	if e.Name == "production" {
		e.ApprovalRequired = true
	}
	if e.RolloutTimeoutSeconds == 0 {
		e.RolloutTimeoutSeconds = 600
	}
	return s.store.CreateEnvironment(ctx, e)
}
func (s *Service) RequestOperation(ctx context.Context, requester uuid.UUID, kind domain.OperationKind, envID uuid.UUID, image string, revision int64, key string) (domain.Operation, error) {
	env, err := s.store.GetEnvironment(ctx, envID)
	if err != nil {
		return domain.Operation{}, err
	}
	if kind == domain.OperationDeploy && !strings.HasPrefix(image, env.ImagePrefix) {
		return domain.Operation{}, fmt.Errorf("image must start with %s", env.ImagePrefix)
	}
	status := domain.StatusQueued
	if env.ApprovalRequired {
		status = domain.StatusPendingApproval
	}
	return s.store.CreateOperation(ctx, domain.Operation{Kind: kind, Status: status, ApplicationID: env.ApplicationID, EnvironmentID: env.ID, RequesterID: requester, Image: image, Revision: revision, IdempotencyKey: key})
}
func (s *Service) Decide(ctx context.Context, operationID, approverID uuid.UUID, decision, comment string) error {
	if decision != "approved" && decision != "rejected" {
		return fmt.Errorf("invalid decision")
	}
	return s.store.Approve(ctx, operationID, approverID, decision, comment)
}
func OperationTimeout(env domain.AppEnvironment) time.Duration {
	return time.Duration(env.RolloutTimeoutSeconds) * time.Second
}
