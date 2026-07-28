package postgres

import (
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"time"
)

type ClusterModel struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name                string
	APIServer           string
	EncryptedKubeconfig []byte
	CredentialVersion   int64
	Enabled             bool
	LastCheckStatus     string
	LastCheckedAt       *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (ClusterModel) TableName() string { return "clusters" }

type ApplicationModel struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name        string
	Description string
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (ApplicationModel) TableName() string { return "applications" }

type EnvironmentModel struct {
	ID                    uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ApplicationID         uuid.UUID
	Name                  string
	ClusterID             uuid.UUID
	Namespace             string
	Deployment            string
	Container             string
	ImagePrefix           string
	ApprovalRequired      bool
	RolloutTimeoutSeconds int
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (EnvironmentModel) TableName() string { return "app_environments" }

type UserModel struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	FeishuOpenID string
	DisplayName  string
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (UserModel) TableName() string { return "users" }

type RoleModel struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name        string
	Description string
	CreatedAt   time.Time
}

func (RoleModel) TableName() string { return "roles" }

type PermissionModel struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	RoleID        uuid.UUID
	ApplicationID *uuid.UUID
	Action        string
}

func (PermissionModel) TableName() string { return "app_permissions" }

type APITokenModel struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name        string
	Fingerprint string
	SecretHash  string
	RoleID      uuid.UUID
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}

func (APITokenModel) TableName() string { return "api_tokens" }

type WebSessionModel struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	SessionHash string
	UserID      uuid.UUID
	CSRFHash    string
	ExpiresAt   time.Time
	LastSeenAt  time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}

func (WebSessionModel) TableName() string { return "web_sessions" }

type OAuthStateModel struct {
	StateHash  string `gorm:"primaryKey"`
	ReturnPath string
	ExpiresAt  time.Time
	UsedAt     *time.Time
}

func (OAuthStateModel) TableName() string { return "oauth_states" }

type OperationModel struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Kind           string
	Status         string
	ApplicationID  uuid.UUID
	EnvironmentID  uuid.UUID
	RequesterID    uuid.UUID
	Image          string
	Revision       int64
	IdempotencyKey string
	ErrorCode      string
	ErrorMessage   string
	Attempts       int
	NextAttemptAt  time.Time
	ClaimedBy      *string
	ClaimedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (OperationModel) TableName() string { return "operations" }

type ApprovalModel struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OperationID uuid.UUID
	ApproverID  uuid.UUID
	Decision    string
	Comment     string
	CreatedAt   time.Time
}

func (ApprovalModel) TableName() string { return "approvals" }

type OutboxModel struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Topic         string
	Destination   string
	Payload       datatypes.JSON
	Attempts      int
	NextAttemptAt time.Time
	SentAt        *time.Time
	LastError     string
	CreatedAt     time.Time
}

func (OutboxModel) TableName() string { return "outbox_messages" }

type OperationEventModel struct {
	ID            int64 `gorm:"primaryKey"`
	OperationID   *uuid.UUID
	ApplicationID *uuid.UUID
	Kind          string
	Payload       datatypes.JSON
	CreatedAt     time.Time
}

func (OperationEventModel) TableName() string { return "operation_events" }
