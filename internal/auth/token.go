package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/store/postgres"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
	"gorm.io/gorm"
)

type Service struct{ db *gorm.DB }

func New(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) EnsureBootstrap(ctx context.Context, plaintext string) error {
	if plaintext == "" {
		return nil
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&postgres.APITokenModel{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	var role postgres.RoleModel
	if err := s.db.WithContext(ctx).Where("name = ?", "admin").First(&role).Error; err != nil {
		return err
	}
	hash, err := hashSecret(plaintext)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(&postgres.APITokenModel{Name: "bootstrap-admin", Fingerprint: fingerprint(plaintext), SecretHash: hash, RoleID: role.ID}).Error
}

type IssuedToken struct {
	ID          uuid.UUID `json:"id"`
	Plaintext   string    `json:"token"`
	Fingerprint string    `json:"fingerprint"`
}

func (s *Service) Issue(ctx context.Context, name string, roleID uuid.UUID, expiresAt *time.Time) (IssuedToken, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return IssuedToken{}, err
	}
	plain := "cdp_" + base64.RawURLEncoding.EncodeToString(raw)
	hash, err := hashSecret(plain)
	if err != nil {
		return IssuedToken{}, err
	}
	row := postgres.APITokenModel{Name: name, Fingerprint: fingerprint(plain), SecretHash: hash, RoleID: roleID, ExpiresAt: expiresAt}
	if err = s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return IssuedToken{}, err
	}
	return IssuedToken{ID: row.ID, Plaintext: plain, Fingerprint: row.Fingerprint}, nil
}
func (s *Service) Revoke(ctx context.Context, id uuid.UUID) error {
	return s.db.WithContext(ctx).Model(&postgres.APITokenModel{}).Where("id = ?", id).Update("revoked_at", time.Now()).Error
}
func (s *Service) Verify(ctx context.Context, plain string) (domain.Principal, error) {
	var token postgres.APITokenModel
	if err := s.db.WithContext(ctx).Where("fingerprint = ?", fingerprint(plain)).First(&token).Error; err != nil {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	if token.RevokedAt != nil || (token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now())) || !verifySecret(plain, token.SecretHash) {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	var perms []postgres.PermissionModel
	if err := s.db.WithContext(ctx).Where("role_id = ?", token.RoleID).Find(&perms).Error; err != nil {
		return domain.Principal{}, err
	}
	out := domain.Principal{TokenID: &token.ID}
	for _, p := range perms {
		out.Permissions = append(out.Permissions, domain.Permission{ID: p.ID, RoleID: p.RoleID, ApplicationID: p.ApplicationID, Action: domain.Action(p.Action)})
	}
	var role postgres.RoleModel
	if err := s.db.WithContext(ctx).First(&role, "id = ?", token.RoleID).Error; err == nil && role.Name == "admin" {
		out.Permissions = append(out.Permissions, domain.Permission{Action: domain.ActionAdmin})
	}
	return out, nil
}

func fingerprint(v string) string { sum := sha256.Sum256([]byte(v)); return hex.EncodeToString(sum[:]) }
func hashSecret(secret string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(secret), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}
func verifySecret(secret, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	expected, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil {
		return false
	}
	actual := argon2.IDKey([]byte(secret), salt, 3, 64*1024, 2, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

type Session struct {
	Cookie string
	CSRF   string
	User   domain.User
}

func (s *Service) CreateSession(ctx context.Context, user domain.User, ttl time.Duration) (Session, error) {
	sessionRaw := make([]byte, 32)
	csrfRaw := make([]byte, 24)
	if _, err := rand.Read(sessionRaw); err != nil {
		return Session{}, err
	}
	if _, err := rand.Read(csrfRaw); err != nil {
		return Session{}, err
	}
	cookie := base64.RawURLEncoding.EncodeToString(sessionRaw)
	csrf := base64.RawURLEncoding.EncodeToString(csrfRaw)
	row := postgres.WebSessionModel{SessionHash: fingerprint(cookie), UserID: user.ID, CSRFHash: fingerprint(csrf), ExpiresAt: time.Now().Add(ttl), LastSeenAt: time.Now()}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return Session{}, err
	}
	return Session{Cookie: cookie, CSRF: csrf, User: user}, nil
}
func (s *Service) VerifySession(ctx context.Context, cookie, csrf string, requireCSRF bool) (domain.Principal, error) {
	var session postgres.WebSessionModel
	if err := s.db.WithContext(ctx).Where("session_hash = ? AND revoked_at IS NULL AND expires_at > now()", fingerprint(cookie)).First(&session).Error; err != nil {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	if requireCSRF && (csrf == "" || subtle.ConstantTimeCompare([]byte(fingerprint(csrf)), []byte(session.CSRFHash)) != 1) {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	var permissions []postgres.PermissionModel
	if err := s.db.WithContext(ctx).Raw("SELECT p.* FROM app_permissions p JOIN user_roles ur ON ur.role_id = p.role_id WHERE ur.user_id = ?", session.UserID).Scan(&permissions).Error; err != nil {
		return domain.Principal{}, err
	}
	principal := domain.Principal{UserID: &session.UserID}
	for _, p := range permissions {
		principal.Permissions = append(principal.Permissions, domain.Permission{ID: p.ID, RoleID: p.RoleID, ApplicationID: p.ApplicationID, Action: domain.Action(p.Action)})
	}
	var adminCount int64
	s.db.WithContext(ctx).Raw("SELECT count(*) FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=? AND r.name='admin'", session.UserID).Scan(&adminCount)
	if adminCount > 0 {
		principal.Permissions = append(principal.Permissions, domain.Permission{Action: domain.ActionAdmin})
	}
	s.db.WithContext(ctx).Model(&session).Update("last_seen_at", time.Now())
	return principal, nil
}
