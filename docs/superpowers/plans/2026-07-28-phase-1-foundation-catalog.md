# Phase 1 Foundation and Catalog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a runnable, tested Go service with configuration, logging, PostgreSQL migrations, core domain rules, API Token authentication, and the first complete administration API for registered clusters, applications, environments, users, roles, and permissions.

**Architecture:** Keep domain rules independent of Gin, GORM, and PostgreSQL. Application services depend on narrow repository and transaction interfaces; GORM and Gin implement those ports. Use explicit Goose SQL migrations and reserve PostgreSQL integration tests for behaviors that cannot be proven with unit tests.

**Tech Stack:** Go 1.22+, Gin, GORM, PostgreSQL, Goose, Viper, Zap, Argon2id, Testify, OpenAPI 3

---

## File Map

```text
go.mod                                      # Module identity and pinned dependencies
cmd/server/main.go                          # Composition root, signal handling, HTTP lifecycle
config/config.example.yaml                  # Non-secret defaults
internal/config/config.go                   # Viper loading and semantic validation
internal/config/config_test.go              # Config defaults, env override, invalid config
internal/logging/logger.go                   # Zap production/development constructors
internal/security/secretbox.go               # Versioned AES-256-GCM encryption envelope
internal/security/secretbox_test.go           # Round-trip, AAD and malformed ciphertext tests
internal/domain/catalog.go                  # Cluster, application and environment entities
internal/domain/identity.go                 # User, role and permission entities
internal/domain/operation.go                # Operation states and legal transitions
internal/domain/operation_test.go            # State-machine tests
internal/domain/permission.go                # Permission constants and scope matching
internal/domain/permission_test.go           # RBAC rule tests
migrations/embed.go                         # Embedded migration filesystem
migrations/000001_core.up.sql               # Phase 1 PostgreSQL schema
migrations/000001_core.down.sql             # Exact reverse migration
internal/store/postgres/db.go                # GORM connection and pool settings
internal/store/postgres/migrate.go           # Goose migration runner
internal/store/postgres/models.go            # Persistence-only GORM models
internal/store/postgres/catalog_store.go     # Catalog and identity repositories
internal/store/postgres/catalog_store_test.go # Real PostgreSQL constraints and transactions
internal/application/ports.go                # Repository, clock and transaction interfaces
internal/application/catalog.go              # Catalog and RBAC administration use cases
internal/application/catalog_test.go         # Use-case tests with fakes
internal/application/token.go                # Token issue, fingerprint, Argon2id verify and revoke
internal/application/token_test.go           # Token lifecycle tests
internal/transport/http/router.go             # Gin engine and route registration
internal/transport/http/respond.go            # Stable success/error envelopes
internal/transport/http/middleware/request_id.go # Request ID validation and propagation
internal/transport/http/middleware/auth.go    # Bearer Token authentication
internal/transport/http/handler/health.go     # Liveness and readiness
internal/transport/http/handler/catalog.go    # Catalog, identity and permission endpoints
internal/transport/http/handler/catalog_test.go # Handler contract tests
api/openapi.yaml                             # Phase 1 API contract
.env.example                                 # Secret variable names without values
compose.yaml                                 # Local PostgreSQL only
Makefile                                     # Repeatable build, migration and test commands
README.md                                    # Phase 1 local run and bootstrap instructions
```

### Task 1: Bootstrap the Go Service, Configuration, Logging, and Health Routes

**Files:**
- Create: `go.mod`
- Create: `cmd/server/main.go`
- Create: `config/config.example.yaml`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`
- Create: `internal/logging/logger.go`
- Create: `internal/transport/http/router.go`
- Create: `internal/transport/http/respond.go`
- Create: `internal/transport/http/handler/health.go`
- Test: `internal/transport/http/handler/health_test.go`

- [ ] **Step 1: Initialize the module and pin Go 1.22-compatible dependencies**

```powershell
go mod init chatops-deploy
go get github.com/gin-gonic/gin@v1.10.1
go get github.com/spf13/viper@v1.20.1
go get go.uber.org/zap@v1.27.0
go get github.com/stretchr/testify@v1.10.0
go mod tidy
```

Expected: `go.mod` declares `module chatops-deploy`, `go 1.22.0`, and `go.sum` is created.

- [ ] **Step 2: Write failing configuration tests**

```go
func TestLoadUsesDefaultsAndEnvironment(t *testing.T) {
    t.Setenv("CHATOPS_HTTP_ADDR", ":9090")
    t.Setenv("CHATOPS_DATABASE_URL", "postgres://user:pass@db/chatops?sslmode=disable")
    cfg, err := Load()
    require.NoError(t, err)
    assert.Equal(t, ":9090", cfg.HTTP.Addr)
    assert.Equal(t, 15*time.Second, cfg.HTTP.ShutdownTimeout)
    assert.Equal(t, "postgres://user:pass@db/chatops?sslmode=disable", cfg.Database.URL)
}

func TestLoadRejectsMissingDatabaseURL(t *testing.T) {
    t.Setenv("CHATOPS_DATABASE_URL", "")
    _, err := Load()
    assert.ErrorContains(t, err, "database.url is required")
}
```

- [ ] **Step 3: Run the focused test and verify failure**

```powershell
go test ./internal/config -run TestLoad -v
```

Expected: FAIL because `Load` and `Config` do not exist.

- [ ] **Step 4: Implement typed Viper configuration**

```go
type Config struct {
    Mode     string         `mapstructure:"mode"`
    HTTP     HTTPConfig     `mapstructure:"http"`
    Database DatabaseConfig `mapstructure:"database"`
    Log      LogConfig      `mapstructure:"log"`
}

type HTTPConfig struct {
    Addr            string        `mapstructure:"addr"`
    ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type DatabaseConfig struct {
    URL             string        `mapstructure:"url"`
    MaxOpenConns    int           `mapstructure:"max_open_conns"`
    MaxIdleConns    int           `mapstructure:"max_idle_conns"`
    ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

type LogConfig struct {
    Level       string `mapstructure:"level"`
    Development bool   `mapstructure:"development"`
}

func Load() (Config, error) {
    v := viper.New()
    v.SetEnvPrefix("CHATOPS")
    v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
    v.AutomaticEnv()
    v.SetDefault("mode", "all")
    v.SetDefault("http.addr", ":8080")
    v.SetDefault("http.shutdown_timeout", "15s")
    v.SetDefault("database.max_open_conns", 20)
    v.SetDefault("database.max_idle_conns", 5)
    v.SetDefault("database.conn_max_lifetime", "30m")
    v.SetDefault("log.level", "info")
    var cfg Config
    if err := v.Unmarshal(&cfg); err != nil { return Config{}, fmt.Errorf("decode config: %w", err) }
    if cfg.Database.URL == "" { return Config{}, errors.New("database.url is required") }
    if cfg.Mode != "all" && cfg.Mode != "api" && cfg.Mode != "worker" {
        return Config{}, fmt.Errorf("mode must be all, api, or worker: %q", cfg.Mode)
    }
    return cfg, nil
}
```

Also configure Viper to read `CHATOPS_CONFIG` when supplied, otherwise attempt `config.yaml` without requiring it. Put only non-secret values in `config/config.example.yaml`.

- [ ] **Step 5: Write health handler tests**

```go
func TestHealthz(t *testing.T) {
    r := NewRouter(Dependencies{Ready: func(context.Context) error { return nil }})
    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
    r.ServeHTTP(w, req)
    assert.Equal(t, http.StatusOK, w.Code)
    assert.JSONEq(t, `{"data":{"status":"ok"}}`, w.Body.String())
}

func TestReadyzReturnsUnavailable(t *testing.T) {
    r := NewRouter(Dependencies{Ready: func(context.Context) error { return errors.New("db down") }})
    w := httptest.NewRecorder()
    r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
    assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
```

- [ ] **Step 6: Implement the response envelope, health handlers, router, logger, and main lifecycle**

Use the response contract below everywhere:

```go
type Envelope struct {
    Data  any       `json:"data,omitempty"`
    Error *APIError `json:"error,omitempty"`
}

type APIError struct {
    Code      string `json:"code"`
    Message   string `json:"message"`
    RequestID string `json:"request_id,omitempty"`
}
```

`main.go` must load config, construct Zap, open dependencies, start `http.Server`, listen for `SIGINT/SIGTERM`, call `Shutdown` with `cfg.HTTP.ShutdownTimeout`, and return non-zero on startup or shutdown failure. Do not use package-global mutable dependencies.

- [ ] **Step 7: Run focused and package tests**

```powershell
go test ./internal/config/... ./internal/transport/http/... -race
go test ./... -race
```

Expected: PASS.

- [ ] **Step 8: Commit the bootstrap**

```powershell
git add go.mod go.sum cmd config internal/config internal/logging internal/transport/http
git commit -m "feat: bootstrap chatops service"
```

### Task 2: Define Domain Models, State Transitions, and RBAC Rules

**Files:**
- Create: `internal/domain/catalog.go`
- Create: `internal/domain/identity.go`
- Create: `internal/domain/operation.go`
- Test: `internal/domain/operation_test.go`
- Create: `internal/domain/permission.go`
- Test: `internal/domain/permission_test.go`

- [ ] **Step 1: Write failing operation transition tests**

```go
func TestOperationCanTransitionTo(t *testing.T) {
    tests := []struct{ from, to OperationStatus; allowed bool }{
        {StatusPendingApproval, StatusApproved, true},
        {StatusPendingApproval, StatusRejected, true},
        {StatusPendingApproval, StatusExpired, true},
        {StatusApproved, StatusQueued, true},
        {StatusQueued, StatusRunning, true},
        {StatusRunning, StatusSucceeded, true},
        {StatusRunning, StatusFailed, true},
        {StatusSucceeded, StatusRunning, false},
        {StatusRejected, StatusQueued, false},
    }
    for _, tt := range tests {
        assert.Equal(t, tt.allowed, tt.from.CanTransitionTo(tt.to), "%s -> %s", tt.from, tt.to)
    }
}
```

- [ ] **Step 2: Run the state test and verify failure**

```powershell
go test ./internal/domain -run TestOperationCanTransitionTo -v
```

Expected: FAIL because the status type is undefined.

- [ ] **Step 3: Implement exact operation types and transitions**

```go
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

var transitions = map[OperationStatus]map[OperationStatus]struct{}{
    StatusPendingApproval: {StatusApproved: {}, StatusRejected: {}, StatusExpired: {}},
    StatusApproved:        {StatusQueued: {}},
    StatusQueued:          {StatusRunning: {}},
    StatusRunning:         {StatusSucceeded: {}, StatusFailed: {}},
}

func (s OperationStatus) CanTransitionTo(next OperationStatus) bool {
    _, ok := transitions[s][next]
    return ok
}
```

Define UUID-backed `User`, `Role`, `Cluster`, `Application`, `AppEnvironment`, `Operation`, `Approval`, and `AuditEvent` entities. Domain structs use `time.Time`, not GORM tags. `AppEnvironment.RequiresApproval()` returns true only for production policy, and `Approval.Validate(requesterID, approverID)` returns `ErrSelfApproval` when the IDs match.

- [ ] **Step 4: Write failing permission tests**

```go
func TestPermissionAllowsAppScope(t *testing.T) {
    p := Permission{Action: ActionDeploy, ApplicationID: appID}
    assert.True(t, p.Allows(ActionDeploy, appID))
    assert.False(t, p.Allows(ActionApprove, appID))
    assert.False(t, p.Allows(ActionDeploy, otherAppID))
}

func TestGlobalAdminPermissionAllowsEveryApplication(t *testing.T) {
    p := Permission{Action: ActionAdmin}
    assert.True(t, p.Allows(ActionDeploy, appID))
    assert.True(t, p.Allows(ActionApprove, otherAppID))
}
```

- [ ] **Step 5: Implement permission actions and matching**

```go
type Action string
const (
    ActionView     Action = "view"
    ActionDeploy   Action = "deploy"
    ActionRollback Action = "rollback"
    ActionApprove  Action = "approve"
    ActionAdmin    Action = "admin"
)

func (p Permission) Allows(action Action, appID uuid.UUID) bool {
    if p.Action == ActionAdmin { return true }
    return p.Action == action && p.ApplicationID != nil && *p.ApplicationID == appID
}
```

- [ ] **Step 6: Run all domain tests and commit**

```powershell
go test ./internal/domain -race
git add internal/domain
git commit -m "feat: define chatops domain rules"
```

Expected: PASS, followed by one domain commit.

### Task 3: Add Explicit PostgreSQL Migrations and Database Lifecycle

**Files:**
- Create: `migrations/embed.go`
- Create: `migrations/000001_core.up.sql`
- Create: `migrations/000001_core.down.sql`
- Create: `internal/store/postgres/db.go`
- Create: `internal/store/postgres/migrate.go`
- Test: `internal/store/postgres/migrate_test.go`
- Create: `compose.yaml`

- [ ] **Step 1: Add persistence dependencies and local PostgreSQL**

```powershell
go get gorm.io/gorm@v1.30.5
go get gorm.io/driver/postgres@v1.5.11
go get github.com/pressly/goose/v3@v3.24.1
go mod tidy
```

Use `postgres:17-alpine` in `compose.yaml`, expose `5432`, create database/user `chatops`, and use a named volume. The healthcheck must run `pg_isready -U chatops -d chatops`.

- [ ] **Step 2: Write the migration with explicit constraints**

The up migration must create, in dependency order:

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE users (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), feishu_open_id text NOT NULL UNIQUE, display_name text NOT NULL, enabled boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE roles (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL UNIQUE, description text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE user_roles (user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE, PRIMARY KEY (user_id, role_id));
CREATE TABLE api_tokens (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL, fingerprint char(64) NOT NULL UNIQUE, secret_hash text NOT NULL, role_id uuid NOT NULL REFERENCES roles(id), expires_at timestamptz, revoked_at timestamptz, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE clusters (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL UNIQUE, api_server text NOT NULL, encrypted_kubeconfig bytea NOT NULL, credential_version bigint NOT NULL DEFAULT 1 CHECK (credential_version > 0), enabled boolean NOT NULL DEFAULT true, last_check_status text NOT NULL DEFAULT 'unknown', last_checked_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE applications (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL UNIQUE, description text NOT NULL DEFAULT '', enabled boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE app_environments (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), application_id uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE, name text NOT NULL CHECK (name IN ('development','test','production')), cluster_id uuid NOT NULL REFERENCES clusters(id), namespace text NOT NULL, deployment text NOT NULL, container text NOT NULL, image_prefix text NOT NULL, approval_required boolean NOT NULL, rollout_timeout_seconds integer NOT NULL DEFAULT 600 CHECK (rollout_timeout_seconds BETWEEN 30 AND 3600), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(application_id, name));
CREATE TABLE app_permissions (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE, application_id uuid REFERENCES applications(id) ON DELETE CASCADE, action text NOT NULL CHECK (action IN ('view','deploy','rollback','approve','admin')), created_at timestamptz NOT NULL DEFAULT now(), UNIQUE NULLS NOT DISTINCT(role_id, application_id, action));
CREATE TABLE audit_events (id bigserial PRIMARY KEY, actor_user_id uuid REFERENCES users(id), actor_token_id uuid REFERENCES api_tokens(id), action text NOT NULL, resource_type text NOT NULL, resource_id uuid, request_id text NOT NULL, details jsonb NOT NULL DEFAULT '{}'::jsonb, created_at timestamptz NOT NULL DEFAULT now(), CHECK ((actor_user_id IS NULL) <> (actor_token_id IS NULL)));
CREATE INDEX audit_events_created_at_idx ON audit_events(created_at DESC);
CREATE INDEX audit_events_resource_idx ON audit_events(resource_type, resource_id, created_at DESC);
```

The down migration drops these tables in exact reverse dependency order and does not drop `pgcrypto`, because the extension may be shared.

- [ ] **Step 3: Write a failing migration integration test**

```go
func TestMigrateUpAndDown(t *testing.T) {
    url := os.Getenv("TEST_DATABASE_URL")
    if url == "" { t.Skip("TEST_DATABASE_URL is not set") }
    db := openTestDB(t, url)
    require.NoError(t, MigrateTo(context.Background(), db, 1))
    assertTableExists(t, db, "app_environments")
    require.NoError(t, MigrateTo(context.Background(), db, 0))
    assertTableMissing(t, db, "app_environments")
}
```

- [ ] **Step 4: Implement embedded Goose migrations and GORM pool setup**

```go
// migrations/embed.go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

`postgres.Open` must configure `MaxOpenConns`, `MaxIdleConns`, and `ConnMaxLifetime`, ping with a bounded context, and return both `*gorm.DB` and a close function. `MigrateUp` must call Goose with the embedded filesystem and PostgreSQL dialect.

- [ ] **Step 5: Run migration tests against PostgreSQL**

```powershell
docker compose up -d postgres
$env:TEST_DATABASE_URL='postgres://chatops:chatops@localhost:5432/chatops?sslmode=disable'
go test ./internal/store/postgres -run TestMigrate -v
```

Expected: PASS and the down migration leaves no Phase 1 tables.

- [ ] **Step 6: Commit database lifecycle**

```powershell
git add go.mod go.sum compose.yaml migrations internal/store/postgres/db.go internal/store/postgres/migrate.go internal/store/postgres/migrate_test.go
git commit -m "feat: add versioned postgres schema"
```

### Task 4: Implement Catalog Repositories and Transaction Boundaries

**Files:**
- Create: `internal/application/ports.go`
- Create: `internal/security/secretbox.go`
- Test: `internal/security/secretbox_test.go`
- Create: `internal/store/postgres/models.go`
- Create: `internal/store/postgres/catalog_store.go`
- Test: `internal/store/postgres/catalog_store_test.go`

- [ ] **Step 1: Write failing kubeconfig encryption tests**

```go
func TestSecretBoxRoundTripAndAADBinding(t *testing.T) {
    key := bytes.Repeat([]byte{0x42}, 32)
    box, err := NewSecretBox(key)
    require.NoError(t, err)
    ciphertext, err := box.Seal([]byte("apiVersion: v1"), []byte("cluster-id:1"))
    require.NoError(t, err)
    plaintext, err := box.Open(ciphertext, []byte("cluster-id:1"))
    require.NoError(t, err)
    assert.Equal(t, "apiVersion: v1", string(plaintext))
    _, err = box.Open(ciphertext, []byte("cluster-id:2"))
    assert.ErrorIs(t, err, ErrInvalidCiphertext)
}
```

- [ ] **Step 2: Implement a versioned AES-256-GCM envelope**

`NewSecretBox` accepts exactly 32 decoded key bytes. `Seal` generates a fresh nonce with `crypto/rand` and returns `[version=1][nonce][ciphertext+tag]`. `Open` rejects unknown versions, short input, authentication failure, or empty AAD as `ErrInvalidCiphertext`. Neither error nor log output may include plaintext, key, nonce, or ciphertext.

Use AAD formatted as `<cluster UUID>:<credential version>` so ciphertext cannot be moved between cluster rows or credential versions.

- [ ] **Step 3: Define the application ports**

```go
type CatalogRepository interface {
    CreateCluster(context.Context, domain.Cluster) (domain.Cluster, error)
    ListClusters(context.Context) ([]domain.Cluster, error)
    CreateApplication(context.Context, domain.Application) (domain.Application, error)
    ListApplications(context.Context) ([]domain.Application, error)
    CreateEnvironment(context.Context, domain.AppEnvironment) (domain.AppEnvironment, error)
    CreateUser(context.Context, domain.User) (domain.User, error)
    CreateRole(context.Context, domain.Role) (domain.Role, error)
    GrantPermission(context.Context, domain.Permission) (domain.Permission, error)
}

type AuditRepository interface { Append(context.Context, domain.AuditEvent) error }

type TransactionManager interface {
    WithinTransaction(context.Context, func(context.Context) error) error
}

type KubeconfigProtector interface {
    Seal(plaintext, additionalData []byte) ([]byte, error)
    Open(ciphertext, additionalData []byte) ([]byte, error)
}
```

Repository methods must translate PostgreSQL unique violations to `domain.ErrConflict` and missing rows to `domain.ErrNotFound`; application code must never inspect driver error strings.

- [ ] **Step 4: Write failing integration tests for constraints and rollback**

```go
func TestCatalogStoreRejectsDuplicateApplicationName(t *testing.T) {
    store := newMigratedStore(t)
    _, err := store.CreateApplication(ctx, domain.Application{Name: "orders", Enabled: true})
    require.NoError(t, err)
    _, err = store.CreateApplication(ctx, domain.Application{Name: "orders", Enabled: true})
    assert.ErrorIs(t, err, domain.ErrConflict)
}

func TestWithinTransactionRollsBackCatalogAndAudit(t *testing.T) {
    store := newMigratedStore(t)
    err := store.WithinTransaction(ctx, func(txCtx context.Context) error {
        _, err := store.CreateApplication(txCtx, domain.Application{Name: "billing", Enabled: true})
        require.NoError(t, err)
        return errors.New("force rollback")
    })
    require.Error(t, err)
    assertApplicationMissing(t, store, "billing")
}
```

- [ ] **Step 5: Implement GORM models and mappers**

Persistence structs must contain GORM tags and nullable SQL types; domain structs must remain free of ORM concerns. Implement explicit `toDomain` and `fromDomain` functions for every entity. Never serialize encrypted kubeconfig in a list response.

- [ ] **Step 6: Implement repository methods and transaction context**

Store the transaction-scoped `*gorm.DB` under a private typed context key. Every repository method obtains the current DB through one helper:

```go
func (s *Store) db(ctx context.Context) *gorm.DB {
    if tx, ok := ctx.Value(txContextKey{}).(*gorm.DB); ok { return tx.WithContext(ctx) }
    return s.dbRoot.WithContext(ctx)
}
```

`WithinTransaction` must preserve context cancellation and return the original application error after rollback.

- [ ] **Step 7: Run security and repository tests and commit**

```powershell
go test ./internal/security ./internal/store/postgres -run 'TestSecretBox|TestCatalog|TestWithinTransaction' -race -v
git add internal/application/ports.go internal/security internal/store/postgres/models.go internal/store/postgres/catalog_store.go internal/store/postgres/catalog_store_test.go
git commit -m "feat: encrypt and persist cluster catalog"
```

Expected: PASS.

### Task 5: Add Secure API Token Issuance and Authentication

**Files:**
- Create: `internal/application/token.go`
- Test: `internal/application/token_test.go`
- Create: `internal/transport/http/middleware/request_id.go`
- Test: `internal/transport/http/middleware/request_id_test.go`
- Create: `internal/transport/http/middleware/auth.go`
- Test: `internal/transport/http/middleware/auth_test.go`

- [ ] **Step 1: Add cryptographic dependency and write failing token tests**

```powershell
go get golang.org/x/crypto@v0.33.0
```

```go
func TestIssueTokenReturnsPlaintextOnceAndStoresNoPlaintext(t *testing.T) {
    repo := &fakeTokenRepo{}
    svc := NewTokenService(repo, fixedClock)
    issued, err := svc.Issue(ctx, "automation", adminRoleID, time.Hour)
    require.NoError(t, err)
    assert.Regexp(t, `^cdp_[A-Za-z0-9_-]{43}$`, issued.Plaintext)
    assert.NotContains(t, repo.saved.SecretHash, issued.Plaintext)
    assert.Len(t, repo.saved.Fingerprint, 64)
    assert.True(t, svc.VerifyHash(issued.Plaintext, repo.saved.SecretHash))
}
```

- [ ] **Step 2: Run the token test and verify failure**

```powershell
go test ./internal/application -run TestIssueToken -v
```

Expected: FAIL because `TokenService` is undefined.

- [ ] **Step 3: Implement token generation, SHA-256 fingerprinting, and Argon2id encoding**

Generate 32 random bytes with `crypto/rand`, encode with raw URL-safe base64, prefix with `cdp_`, compute the lowercase hex SHA-256 fingerprint, and encode the verifier as:

```text
$argon2id$v=19$m=65536,t=3,p=2$<base64-salt>$<base64-hash>
```

Use a fresh 16-byte salt and a 32-byte Argon2id result. Verification must parse fixed parameters, recompute the result, and use `subtle.ConstantTimeCompare`. Reject malformed hashes without panicking.

- [ ] **Step 4: Write request ID and authentication middleware tests**

```go
func TestAuthRejectsRevokedToken(t *testing.T) {
    verifier := fakeVerifier{principal: domain.Principal{}, err: domain.ErrTokenRevoked}
    r := gin.New()
    r.Use(Auth(verifier))
    r.GET("/private", func(c *gin.Context) { c.Status(http.StatusNoContent) })
    w := perform(r, http.MethodGet, "/private", "Bearer cdp_invalid")
    assert.Equal(t, http.StatusUnauthorized, w.Code)
    assert.Contains(t, w.Body.String(), `"code":"unauthorized"`)
}
```

Request IDs may contain only ASCII letters, digits, `_`, `-`, and `.`, with length 8 through 128; otherwise generate a UUID. Always return the accepted/generated ID in `X-Request-ID`.

- [ ] **Step 5: Implement middleware and bootstrap behavior**

Authentication must hash the presented Token to find its record, verify Argon2id, then reject expired, revoked, disabled-role, or missing tokens. Put a `domain.Principal` in Gin context under a private key. Bootstrap logic creates the first admin role and Token only when no API Token exists and `CHATOPS_BOOTSTRAP_ADMIN_TOKEN` is set; it stores only the fingerprint and Argon2id value and logs only the fingerprint.

- [ ] **Step 6: Run authentication tests and commit**

```powershell
go test ./internal/application ./internal/transport/http/middleware -race
git add go.mod go.sum internal/application/token.go internal/application/token_test.go internal/transport/http/middleware
git commit -m "feat: secure admin api with hashed tokens"
```

Expected: PASS.

### Task 6: Implement Catalog Administration Use Cases and Gin Endpoints

**Files:**
- Create: `internal/application/catalog.go`
- Test: `internal/application/catalog_test.go`
- Create: `internal/transport/http/handler/catalog.go`
- Test: `internal/transport/http/handler/catalog_test.go`
- Modify: `internal/transport/http/router.go`

- [ ] **Step 1: Write failing use-case tests for validation, authorization, and atomic audit**

```go
func TestCreateEnvironmentForcesProductionApproval(t *testing.T) {
    svc, repo := newCatalogServiceWithAdmin(t)
    got, err := svc.CreateEnvironment(ctx, adminPrincipal, CreateEnvironmentInput{
        ApplicationID: appID, ClusterID: clusterID, Name: "production",
        Namespace: "orders", Deployment: "orders-api", Container: "api",
        ImagePrefix: "ghcr.io/acme/orders:", RolloutTimeout: 10 * time.Minute,
    })
    require.NoError(t, err)
    assert.True(t, got.ApprovalRequired)
    assert.Equal(t, "environment.created", repo.audit.Action)
}

func TestCreateClusterRejectsNonAdmin(t *testing.T) {
    svc, _ := newCatalogService(t)
    _, err := svc.CreateCluster(ctx, viewerPrincipal, validClusterInput())
    assert.ErrorIs(t, err, domain.ErrForbidden)
}
```

- [ ] **Step 2: Implement input validation and transactional use cases**

Validate DNS-style namespaces and Kubernetes names, normalize application names to lowercase kebab-case, require HTTPS API Server URLs except for explicit local-test configuration, require image prefixes containing a registry and repository, and enforce rollout timeout between 30 seconds and one hour. For cluster creation, generate the cluster UUID and credential version first, encrypt the submitted kubeconfig with AAD `<cluster UUID>:1`, and pass only ciphertext to the repository. Each successful mutation and its `audit_events` row must commit in one transaction.

- [ ] **Step 3: Write failing handler contract tests**

```go
func TestCreateApplicationEndpoint(t *testing.T) {
    r := authenticatedTestRouter(adminPrincipal)
    w := jsonRequest(r, http.MethodPost, "/api/v1/applications", `{"name":"orders","description":"Order API"}`)
    assert.Equal(t, http.StatusCreated, w.Code)
    assert.JSONEq(t, `{"data":{"id":"00000000-0000-0000-0000-000000000111","name":"orders","description":"Order API","enabled":true}}`, w.Body.String())
}

func TestCreateApplicationEndpointRejectsUnknownField(t *testing.T) {
    r := authenticatedTestRouter(adminPrincipal)
    w := jsonRequest(r, http.MethodPost, "/api/v1/applications", `{"name":"orders","admin":true}`)
    assert.Equal(t, http.StatusBadRequest, w.Code)
    assert.Contains(t, w.Body.String(), `"code":"invalid_request"`)
}
```

- [ ] **Step 4: Implement strict JSON handlers and routes**

Register these authenticated endpoints:

```text
GET,POST              /api/v1/clusters
GET,PATCH             /api/v1/clusters/:id
POST                  /api/v1/clusters/:id/disable
GET,POST              /api/v1/applications
GET,PATCH             /api/v1/applications/:id
GET,POST              /api/v1/applications/:id/environments
GET,PATCH              /api/v1/environments/:id
GET,POST              /api/v1/users
GET,PATCH             /api/v1/users/:id
GET,POST              /api/v1/roles
POST,DELETE           /api/v1/roles/:id/users/:user_id
GET,POST              /api/v1/roles/:id/permissions
DELETE                /api/v1/roles/:id/permissions/:permission_id
GET                   /api/v1/audit-events
POST                  /api/v1/api-tokens
DELETE                /api/v1/api-tokens/:id
```

Decode JSON with `DisallowUnknownFields`, cap bodies at 1 MiB, return `201` for creates, `204` for deletes/disable actions, and map domain errors consistently: invalid input `400`, unauthenticated `401`, forbidden `403`, missing `404`, conflict `409`, internal `500`.

- [ ] **Step 5: Run application and handler tests**

```powershell
go test ./internal/application ./internal/transport/http/... -race
```

Expected: PASS.

- [ ] **Step 6: Commit the catalog API**

```powershell
git add internal/application/catalog.go internal/application/catalog_test.go internal/transport/http/router.go internal/transport/http/handler/catalog.go internal/transport/http/handler/catalog_test.go
git commit -m "feat: add catalog administration api"
```

### Task 7: Publish the Phase 1 OpenAPI Contract and Developer Workflow

**Files:**
- Create: `api/openapi.yaml`
- Create: `.env.example`
- Create: `Makefile`
- Create: `README.md`
- Modify: `cmd/server/main.go`
- Test: `internal/transport/http/openapi_contract_test.go`

- [ ] **Step 1: Write a failing route-to-contract test**

```go
func TestEveryAPIRouteExistsInOpenAPI(t *testing.T) {
    spec := loadOpenAPI(t, "../../../api/openapi.yaml")
    router := newFullyWiredTestRouter(t)
    for _, route := range router.Routes() {
        if !strings.HasPrefix(route.Path, "/api/v1/") { continue }
        assertOpenAPIOperation(t, spec, route.Method, route.Path)
    }
}
```

- [ ] **Step 2: Create the OpenAPI 3.1 contract**

Define Bearer authentication, `X-Request-ID`, `Idempotency-Key`, the shared envelope/error schemas, UUID/date-time formats, pagination (`limit`, `cursor`, `next_cursor`), every Phase 1 route, all request schemas with `additionalProperties: false`, and response examples for success plus `400/401/403/404/409/500`.

- [ ] **Step 3: Add local developer commands**

`Makefile` must expose:

```make
fmt:
	go fmt ./...
test:
	go test ./... -race
migrate-up:
	go run ./cmd/server migrate up
run:
	go run ./cmd/server serve
verify: fmt test
```

`README.md` must document prerequisites, `docker compose up -d postgres`, environment setup, bootstrap Token behavior, migrations, `make run`, health checks, API examples, test commands, and the fact that encrypted kubeconfig values are not used to contact a cluster until Phase 2. `.env.example` lists variable names with empty secret values, including `CHATOPS_KUBECONFIG_MASTER_KEY` as a base64-encoded 32-byte key.

- [ ] **Step 4: Wire explicit CLI subcommands**

`cmd/server/main.go` accepts `serve`, `migrate up`, `migrate down`, and `version`. Missing command defaults to `serve`; unknown commands return exit code 2 and usage text. Migration commands never start HTTP or workers.

- [ ] **Step 5: Run full Phase 1 verification**

```powershell
go fmt ./...
go vet ./...
go test ./... -race
go run ./cmd/server version
```

Expected: formatting produces no diff, vet is clean, all tests PASS, and version prints `dev` plus the Go runtime version.

- [ ] **Step 6: Commit documentation and contract**

```powershell
git add api .env.example Makefile README.md cmd/server/main.go internal/transport/http/openapi_contract_test.go
git commit -m "docs: publish phase one api contract"
```

### Task 8: Phase 1 Clean-State Verification

**Files:**
- Modify only files required to fix verification failures; do not expand scope.

- [ ] **Step 1: Recreate the local database from migrations**

```powershell
docker compose down -v
docker compose up -d postgres
$env:TEST_DATABASE_URL='postgres://chatops:chatops@localhost:5432/chatops?sslmode=disable'
go run ./cmd/server migrate up
```

Expected: PostgreSQL becomes healthy and migration version is `1`.

- [ ] **Step 2: Run all checks from a clean Go cache-independent state**

```powershell
go mod tidy
git diff --exit-code -- go.mod go.sum
go vet ./...
go test ./... -count=1 -race
```

Expected: no module drift, vet clean, all tests PASS.

- [ ] **Step 3: Smoke-test the runnable service**

Start `go run ./cmd/server serve` with the example non-secret config and required environment variables, then verify:

```powershell
Invoke-WebRequest http://localhost:8080/healthz -UseBasicParsing
Invoke-WebRequest http://localhost:8080/readyz -UseBasicParsing
```

Expected: both return HTTP 200 with `{"data":{"status":"ok"}}`.

- [ ] **Step 4: Confirm verification made no uncommitted changes**

```powershell
git status --short
```

Expected: no output. If files are listed, return to the task responsible for that file, fix the failing requirement, rerun its focused and broad tests, and use that task's exact commit step. Do not create an empty verification commit.
