package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"chatops-deploy/internal/domain"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct{ DB *gorm.DB }

func New(db *gorm.DB) *Store { return &Store{DB: db} }

func (s *Store) Ready(ctx context.Context) error {
	sqlDB, err := s.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (s *Store) ListClusters(ctx context.Context) ([]domain.Cluster, error) {
	var rows []ClusterModel
	err := s.DB.WithContext(ctx).Order("name").Find(&rows).Error
	out := make([]domain.Cluster, 0, len(rows))
	for _, r := range rows {
		out = append(out, clusterDomain(r))
	}
	return out, err
}
func (s *Store) GetCluster(ctx context.Context, id uuid.UUID) (domain.Cluster, error) {
	var r ClusterModel
	if err := s.DB.WithContext(ctx).First(&r, "id = ?", id).Error; err != nil {
		return domain.Cluster{}, mapError(err)
	}
	return clusterDomain(r), nil
}
func (s *Store) CreateCluster(ctx context.Context, c domain.Cluster) (domain.Cluster, error) {
	r := ClusterModel{ID: c.ID, Name: c.Name, APIServer: c.APIServer, EncryptedKubeconfig: c.EncryptedKubeconfig, CredentialVersion: c.CredentialVersion, Enabled: c.Enabled, LastCheckStatus: "unknown"}
	if err := s.DB.WithContext(ctx).Create(&r).Error; err != nil {
		return c, mapError(err)
	}
	return clusterDomain(r), nil
}
func clusterDomain(r ClusterModel) domain.Cluster {
	return domain.Cluster{ID: r.ID, Name: r.Name, APIServer: r.APIServer, EncryptedKubeconfig: r.EncryptedKubeconfig, CredentialVersion: r.CredentialVersion, Enabled: r.Enabled, LastCheckStatus: r.LastCheckStatus, LastCheckedAt: r.LastCheckedAt}
}

func (s *Store) ListApplications(ctx context.Context) ([]domain.Application, error) {
	var rows []ApplicationModel
	err := s.DB.WithContext(ctx).Order("name").Find(&rows).Error
	out := make([]domain.Application, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Application{ID: r.ID, Name: r.Name, Description: r.Description, Enabled: r.Enabled})
	}
	return out, err
}
func (s *Store) CreateApplication(ctx context.Context, a domain.Application) (domain.Application, error) {
	r := ApplicationModel{Name: a.Name, Description: a.Description, Enabled: true}
	if err := s.DB.WithContext(ctx).Create(&r).Error; err != nil {
		return a, mapError(err)
	}
	a.ID = r.ID
	a.Enabled = r.Enabled
	return a, nil
}
func (s *Store) GetEnvironment(ctx context.Context, id uuid.UUID) (domain.AppEnvironment, error) {
	var r EnvironmentModel
	if err := s.DB.WithContext(ctx).First(&r, "id = ?", id).Error; err != nil {
		return domain.AppEnvironment{}, mapError(err)
	}
	return envDomain(r), nil
}
func (s *Store) ListEnvironments(ctx context.Context, appID *uuid.UUID) ([]domain.AppEnvironment, error) {
	q := s.DB.WithContext(ctx).Order("name")
	if appID != nil {
		q = q.Where("application_id = ?", *appID)
	}
	var rows []EnvironmentModel
	err := q.Find(&rows).Error
	out := make([]domain.AppEnvironment, 0, len(rows))
	for _, r := range rows {
		out = append(out, envDomain(r))
	}
	return out, err
}
func (s *Store) CreateEnvironment(ctx context.Context, e domain.AppEnvironment) (domain.AppEnvironment, error) {
	r := EnvironmentModel{ApplicationID: e.ApplicationID, Name: e.Name, ClusterID: e.ClusterID, Namespace: e.Namespace, Deployment: e.Deployment, Container: e.Container, ImagePrefix: e.ImagePrefix, ApprovalRequired: e.ApprovalRequired, RolloutTimeoutSeconds: e.RolloutTimeoutSeconds}
	if err := s.DB.WithContext(ctx).Create(&r).Error; err != nil {
		return e, mapError(err)
	}
	return envDomain(r), nil
}
func envDomain(r EnvironmentModel) domain.AppEnvironment {
	return domain.AppEnvironment{ID: r.ID, ApplicationID: r.ApplicationID, Name: r.Name, ClusterID: r.ClusterID, Namespace: r.Namespace, Deployment: r.Deployment, Container: r.Container, ImagePrefix: r.ImagePrefix, ApprovalRequired: r.ApprovalRequired, RolloutTimeoutSeconds: r.RolloutTimeoutSeconds}
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	var rows []UserModel
	err := s.DB.WithContext(ctx).Order("display_name").Find(&rows).Error
	out := make([]domain.User, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.User{ID: r.ID, FeishuOpenID: r.FeishuOpenID, DisplayName: r.DisplayName, Enabled: r.Enabled, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
	}
	return out, err
}
func (s *Store) CreateUser(ctx context.Context, u domain.User) (domain.User, error) {
	r := UserModel{FeishuOpenID: u.FeishuOpenID, DisplayName: u.DisplayName, Enabled: true}
	if err := s.DB.WithContext(ctx).Create(&r).Error; err != nil {
		return u, mapError(err)
	}
	u.ID = r.ID
	u.Enabled = true
	return u, nil
}
func (s *Store) ListRoles(ctx context.Context) ([]domain.Role, error) {
	var rows []RoleModel
	err := s.DB.WithContext(ctx).Order("name").Find(&rows).Error
	out := make([]domain.Role, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Role{ID: r.ID, Name: r.Name, Description: r.Description})
	}
	return out, err
}
func (s *Store) CreateRole(ctx context.Context, r domain.Role) (domain.Role, error) {
	row := RoleModel{Name: r.Name, Description: r.Description}
	if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
		return r, mapError(err)
	}
	r.ID = row.ID
	return r, nil
}
func (s *Store) AddUserRole(ctx context.Context, userID, roleID uuid.UUID) error {
	return s.DB.WithContext(ctx).Exec("INSERT INTO user_roles(user_id,role_id) VALUES (?,?) ON CONFLICT DO NOTHING", userID, roleID).Error
}
func (s *Store) GrantPermission(ctx context.Context, p domain.Permission) (domain.Permission, error) {
	row := PermissionModel{RoleID: p.RoleID, ApplicationID: p.ApplicationID, Action: string(p.Action)}
	if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
		return p, mapError(err)
	}
	p.ID = row.ID
	return p, nil
}
func (s *Store) ListPermissions(ctx context.Context, roleID uuid.UUID) ([]domain.Permission, error) {
	var rows []PermissionModel
	err := s.DB.WithContext(ctx).Where("role_id = ?", roleID).Find(&rows).Error
	out := make([]domain.Permission, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Permission{ID: r.ID, RoleID: r.RoleID, ApplicationID: r.ApplicationID, Action: domain.Action(r.Action)})
	}
	return out, err
}
func (s *Store) FindUserByOpenID(ctx context.Context, openID string) (domain.User, error) {
	var r UserModel
	if err := s.DB.WithContext(ctx).Where("feishu_open_id = ? AND enabled", openID).First(&r).Error; err != nil {
		return domain.User{}, mapError(err)
	}
	return domain.User{ID: r.ID, FeishuOpenID: r.FeishuOpenID, DisplayName: r.DisplayName, Enabled: r.Enabled}, nil
}

func (s *Store) PrincipalForUser(ctx context.Context, userID uuid.UUID) (domain.Principal, error) {
	principal := domain.Principal{UserID: &userID}
	var permissions []PermissionModel
	if err := s.DB.WithContext(ctx).Raw("SELECT p.* FROM app_permissions p JOIN user_roles ur ON ur.role_id = p.role_id WHERE ur.user_id = ?", userID).Scan(&permissions).Error; err != nil {
		return principal, err
	}
	for _, permission := range permissions {
		principal.Permissions = append(principal.Permissions, domain.Permission{ID: permission.ID, RoleID: permission.RoleID, ApplicationID: permission.ApplicationID, Action: domain.Action(permission.Action)})
	}
	var adminCount int64
	if err := s.DB.WithContext(ctx).Raw("SELECT count(*) FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=? AND r.name='admin'", userID).Scan(&adminCount).Error; err != nil {
		return principal, err
	}
	if adminCount > 0 {
		principal.Permissions = append(principal.Permissions, domain.Permission{Action: domain.ActionAdmin})
	}
	return principal, nil
}

func (s *Store) CreateOperation(ctx context.Context, o domain.Operation) (domain.Operation, error) {
	r := OperationModel{Kind: string(o.Kind), Status: string(o.Status), ApplicationID: o.ApplicationID, EnvironmentID: o.EnvironmentID, RequesterID: o.RequesterID, Image: o.Image, Revision: o.Revision, IdempotencyKey: o.IdempotencyKey}
	if err := s.DB.WithContext(ctx).Create(&r).Error; err != nil {
		return o, mapError(err)
	}
	o.ID = r.ID
	o.CreatedAt = r.CreatedAt
	o.UpdatedAt = r.UpdatedAt
	_ = s.AppendEvent(ctx, o.ID, &o.ApplicationID, "operation.created", o)
	return o, nil
}
func (s *Store) GetOperation(ctx context.Context, id uuid.UUID) (domain.Operation, error) {
	var r OperationModel
	if err := s.DB.WithContext(ctx).First(&r, "id = ?", id).Error; err != nil {
		return domain.Operation{}, mapError(err)
	}
	return operationDomain(r), nil
}
func (s *Store) ListOperations(ctx context.Context, limit int) ([]domain.Operation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []OperationModel
	err := s.DB.WithContext(ctx).Order("created_at DESC").Limit(limit).Find(&rows).Error
	out := make([]domain.Operation, 0, len(rows))
	for _, r := range rows {
		out = append(out, operationDomain(r))
	}
	return out, err
}
func operationDomain(r OperationModel) domain.Operation {
	return domain.Operation{ID: r.ID, Kind: domain.OperationKind(r.Kind), Status: domain.OperationStatus(r.Status), ApplicationID: r.ApplicationID, EnvironmentID: r.EnvironmentID, RequesterID: r.RequesterID, Image: r.Image, Revision: r.Revision, IdempotencyKey: r.IdempotencyKey, ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func (s *Store) TransitionOperation(ctx context.Context, id uuid.UUID, from, to domain.OperationStatus, code, message string) error {
	if !from.CanTransitionTo(to) {
		return domain.ErrInvalidState
	}
	res := s.DB.WithContext(ctx).Model(&OperationModel{}).Where("id = ? AND status = ?", id, string(from)).Updates(map[string]any{"status": string(to), "error_code": code, "error_message": message, "updated_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return domain.ErrConflict
	}
	o, _ := s.GetOperation(ctx, id)
	return s.AppendEvent(ctx, id, &o.ApplicationID, "operation.status", map[string]any{"status": to})
}
func (s *Store) Approve(ctx context.Context, id, approver uuid.UUID, decision, comment string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var o OperationModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&o, "id = ?", id).Error; err != nil {
			return mapError(err)
		}
		if o.RequesterID == approver {
			return domain.ErrSelfApproval
		}
		to := domain.StatusApproved
		if decision == "rejected" {
			to = domain.StatusRejected
		}
		if !domain.OperationStatus(o.Status).CanTransitionTo(to) {
			return domain.ErrInvalidState
		}
		a := ApprovalModel{OperationID: id, ApproverID: approver, Decision: decision, Comment: comment}
		if err := tx.Create(&a).Error; err != nil {
			return mapError(err)
		}
		if err := tx.Model(&o).Updates(map[string]any{"status": string(to), "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		if to == domain.StatusApproved {
			return tx.Model(&o).Update("status", string(domain.StatusQueued)).Error
		}
		return nil
	})
}
func (s *Store) ClaimOperation(ctx context.Context, owner string) (domain.Operation, error) {
	var row OperationModel
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw("SELECT * FROM operations WHERE status = 'queued' AND next_attempt_at <= now() ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1").Scan(&row).Error; err != nil {
			return err
		}
		if row.ID == uuid.Nil {
			return domain.ErrNotFound
		}
		now := time.Now()
		row.Status = string(domain.StatusRunning)
		row.ClaimedBy = &owner
		row.ClaimedAt = &now
		return tx.Save(&row).Error
	})
	return operationDomain(row), err
}
func (s *Store) AppendEvent(ctx context.Context, opID uuid.UUID, appID *uuid.UUID, kind string, payload any) error {
	body, _ := json.Marshal(payload)
	return s.DB.WithContext(ctx).Create(&OperationEventModel{OperationID: &opID, ApplicationID: appID, Kind: kind, Payload: datatypes.JSON(body)}).Error
}

func (s *Store) RecentEvents(ctx context.Context, after int64, limit int) ([]OperationEventModel, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []OperationEventModel
	err := s.DB.WithContext(ctx).Where("id > ? AND created_at > now() - interval '24 hours'", after).Order("id").Limit(limit).Find(&rows).Error
	return rows, err
}

func mapError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	return err
}
