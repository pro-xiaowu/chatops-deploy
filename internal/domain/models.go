package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrForbidden    = errors.New("forbidden")
	ErrUnauthorized = errors.New("unauthorized")
	ErrInvalidState = errors.New("invalid state transition")
	ErrSelfApproval = errors.New("requester cannot approve own operation")
)

type Action string

const (
	ActionView     Action = "view"
	ActionDeploy   Action = "deploy"
	ActionRollback Action = "rollback"
	ActionApprove  Action = "approve"
	ActionAdmin    Action = "admin"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	FeishuOpenID string    `json:"feishu_open_id"`
	DisplayName  string    `json:"display_name"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Role struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
}

type Permission struct {
	ID            uuid.UUID  `json:"id"`
	RoleID        uuid.UUID  `json:"role_id"`
	ApplicationID *uuid.UUID `json:"application_id,omitempty"`
	Action        Action     `json:"action"`
}

func (p Permission) Allows(action Action, appID uuid.UUID) bool {
	if p.Action == ActionAdmin {
		return true
	}
	return p.Action == action && p.ApplicationID != nil && *p.ApplicationID == appID
}

type Principal struct {
	UserID      *uuid.UUID
	TokenID     *uuid.UUID
	Permissions []Permission
}

func (p Principal) Allows(action Action, appID uuid.UUID) bool {
	for _, permission := range p.Permissions {
		if permission.Allows(action, appID) {
			return true
		}
	}
	return false
}

type Cluster struct {
	ID                  uuid.UUID  `json:"id"`
	Name                string     `json:"name"`
	APIServer           string     `json:"api_server"`
	EncryptedKubeconfig []byte     `json:"-"`
	CredentialVersion   int64      `json:"credential_version"`
	Enabled             bool       `json:"enabled"`
	LastCheckStatus     string     `json:"last_check_status"`
	LastCheckedAt       *time.Time `json:"last_checked_at,omitempty"`
}

type Application struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
}

type AppEnvironment struct {
	ID                    uuid.UUID `json:"id"`
	ApplicationID         uuid.UUID `json:"application_id"`
	Name                  string    `json:"name"`
	ClusterID             uuid.UUID `json:"cluster_id"`
	Namespace             string    `json:"namespace"`
	Deployment            string    `json:"deployment"`
	Container             string    `json:"container"`
	ImagePrefix           string    `json:"image_prefix"`
	ApprovalRequired      bool      `json:"approval_required"`
	RolloutTimeoutSeconds int       `json:"rollout_timeout_seconds"`
}

type OperationKind string

const (
	OperationDeploy   OperationKind = "deploy"
	OperationRollback OperationKind = "rollback"
)

type OperationStatus string

const (
	StatusPendingApproval OperationStatus = "pending_approval"
	StatusApproved        OperationStatus = "approved"
	StatusRejected        OperationStatus = "rejected"
	StatusExpired         OperationStatus = "expired"
	StatusQueued          OperationStatus = "queued"
	StatusRunning         OperationStatus = "running"
	StatusSucceeded       OperationStatus = "succeeded"
	StatusFailed          OperationStatus = "failed"
)

var legalTransitions = map[OperationStatus]map[OperationStatus]bool{
	StatusPendingApproval: {StatusApproved: true, StatusRejected: true, StatusExpired: true},
	StatusApproved:        {StatusQueued: true}, StatusQueued: {StatusRunning: true},
	StatusRunning: {StatusSucceeded: true, StatusFailed: true},
}

func (s OperationStatus) CanTransitionTo(next OperationStatus) bool { return legalTransitions[s][next] }

type Operation struct {
	ID             uuid.UUID       `json:"id"`
	Kind           OperationKind   `json:"kind"`
	Status         OperationStatus `json:"status"`
	ApplicationID  uuid.UUID       `json:"application_id"`
	EnvironmentID  uuid.UUID       `json:"environment_id"`
	RequesterID    uuid.UUID       `json:"requester_id"`
	Image          string          `json:"image,omitempty"`
	Revision       int64           `json:"revision,omitempty"`
	IdempotencyKey string          `json:"idempotency_key"`
	ErrorCode      string          `json:"error_code,omitempty"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type Approval struct {
	ID          uuid.UUID `json:"id"`
	OperationID uuid.UUID `json:"operation_id"`
	ApproverID  uuid.UUID `json:"approver_id"`
	Decision    string    `json:"decision"`
	Comment     string    `json:"comment"`
	CreatedAt   time.Time `json:"created_at"`
}

func ValidateApprover(requester, approver uuid.UUID) error {
	if requester == approver {
		return ErrSelfApproval
	}
	return nil
}

type AuditEvent struct {
	ID           int64          `json:"id"`
	ActorUserID  *uuid.UUID     `json:"actor_user_id,omitempty"`
	ActorTokenID *uuid.UUID     `json:"actor_token_id,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   *uuid.UUID     `json:"resource_id,omitempty"`
	RequestID    string         `json:"request_id"`
	Details      map[string]any `json:"details"`
	CreatedAt    time.Time      `json:"created_at"`
}
