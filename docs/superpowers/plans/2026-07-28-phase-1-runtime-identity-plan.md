# Runtime and Identity Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (recommended for this phase) or superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add validated runtime/provider configuration, normalized external identities, persisted active-provider state, operation message origin metadata, and idempotent local administrator provisioning without exposing provider secrets.

**Architecture:** Keep `internal/domain` independent of Gin/GORM. Add provider and identity value types there, map them through explicit PostgreSQL models, and add migration `000002_message_platforms`. The existing Feishu column remains nullable for backward compatibility while all new inbound identities use `user_external_identities`.

**Tech Stack:** Go 1.25, Viper, GORM, PostgreSQL 17, UUIDs, embedded SQL migrations, Testify.

## Global Constraints

- Follow red-green-refactor for every new behavior.
- Do not alter or stage the existing `LICENSE`, `README.md` line-ending changes, or conversation export.
- Provider secrets are configuration inputs only; no secret is stored in the new tables.
- The migration must be forward and backward executable with the existing migration runner.
- Keep existing `FindUserByOpenID` callers working while moving new code to normalized identities.

### Task 1: Add runtime and provider configuration

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`
- Modify: `config/config.example.yaml`
- Modify: `.env.example`

**Interfaces:**
- Produces `Config.RuntimeEnvironment string`, `Config.DevAuthEnabled bool`, and `Config.MessageProvider string`.
- `config.Load()` accepts `CHATOPS_RUNTIME_ENV`, `CHATOPS_DEV_AUTH_ENABLED`, and `CHATOPS_MESSAGE_PROVIDER`.

- [ ] **Step 1: Write the failing tests**

```go
func TestLoadDefaultsToProductionAndWebProvider(t *testing.T) {
    setConfigFile(t, "database:\n  url: postgres://localhost/chatops\nsecurity:\n  kubeconfig_master_key: test-key\n")
    cfg, err := config.Load()
    require.NoError(t, err)
    require.Equal(t, "production", cfg.RuntimeEnvironment)
    require.False(t, cfg.DevAuthEnabled)
    require.Equal(t, "web", cfg.MessageProvider)
}

func TestLoadRejectsDevelopmentAuthInProduction(t *testing.T) {
    setConfigFile(t, "database:\n  url: postgres://localhost/chatops\nsecurity:\n  kubeconfig_master_key: test-key\n")
    t.Setenv("CHATOPS_RUNTIME_ENV", "production")
    t.Setenv("CHATOPS_DEV_AUTH_ENABLED", "true")
    _, err := config.Load()
    require.EqualError(t, err, "development auth requires runtime environment development")
}

func TestLoadRejectsUnknownMessageProvider(t *testing.T) {
    setConfigFile(t, "database:\n  url: postgres://localhost/chatops\nsecurity:\n  kubeconfig_master_key: test-key\n")
    t.Setenv("CHATOPS_MESSAGE_PROVIDER", "telegram")
    _, err := config.Load()
    require.EqualError(t, err, "message provider must be one of web, feishu, wecom, dingtalk")
}
```

- [ ] **Step 2: Run the focused tests and verify the expected failure**

Run: `go test ./internal/config -run 'TestLoad(Default|RejectsDevelopment|RejectsUnknown)' -v`

Expected: FAIL because `Config` has no runtime/provider fields and `Load` does not validate the new values.

- [ ] **Step 3: Implement the minimum configuration behavior**

Add defaults for `runtime_environment=production`, `dev_auth_enabled=false`, and `message_provider=web`. Bind the three environment variables explicitly, validate runtime values, reject development auth in production, and validate the four provider names. Extend `setConfigFile` cleanup in the test to clear the new variables.

- [ ] **Step 4: Run focused and existing configuration tests**

Run: `go test ./internal/config -v`

Expected: PASS with the new tests and all existing default/override/required-field tests.

- [ ] **Step 5: Update safe examples and commit**

Add non-secret keys and comments to `config/config.example.yaml` and `.env.example`, run `git diff --check -- internal/config config/config.example.yaml .env.example`, then commit:

```bash
git add internal/config/config.go internal/config/config_test.go config/config.example.yaml .env.example
git commit -m "feat: validate runtime and message provider settings"
```

### Task 2: Define identity/provider domain values and SQL migration

**Files:**
- Modify: `internal/domain/models.go`
- Modify: `internal/store/postgres/models.go`
- Create: `migrations/000002_message_platforms.up.sql`
- Create: `migrations/000002_message_platforms.down.sql`
- Test: `internal/store/postgres/migration_test.go`

**Interfaces:**
- Produces `domain.MessageProvider`, `domain.ExternalIdentity`, `domain.MessageOrigin`, and provider values `web`, `feishu`, `wecom`, `dingtalk`.
- `Operation` carries `MessageProvider`, `ConversationID`, and `EventID` origin fields.
- `UserModel.FeishuOpenID` becomes nullable-compatible; new identity records use `(provider, subject_id)` uniqueness.

- [ ] **Step 1: Write the failing migration and mapping tests**

```go
func TestMigrationCreatesProviderSettingsAndExternalIdentities(t *testing.T) {
    db := integrationDB(t)
    require.NoError(t, Migrate(context.Background(), db))
    require.True(t, db.Migrator().HasTable("system_settings"))
    require.True(t, db.Migrator().HasTable("user_external_identities"))
    require.NoError(t, db.Exec("SELECT message_provider, message_conversation_id, message_event_id FROM operations LIMIT 1").Error)
}

func TestProviderIdentityValuesRejectUnknownProvider(t *testing.T) {
    require.Error(t, domain.ValidateMessageProvider("telegram"))
    require.NoError(t, domain.ValidateMessageProvider(domain.MessageProviderWeb))
}

func integrationDB(t *testing.T) *gorm.DB {
    t.Helper()
    url := os.Getenv("CHATOPS_TEST_DATABASE_URL")
    if url == "" {
        t.Skip("CHATOPS_TEST_DATABASE_URL is required for PostgreSQL integration tests")
    }
    db, sqlDB, err := Open(context.Background(), url, 4, 2, time.Minute)
    require.NoError(t, err)
    t.Cleanup(func() { _ = sqlDB.Close() })
    return db
}
```

- [ ] **Step 2: Run the tests and observe the missing table/value failure**

Run: `CHATOPS_TEST_DATABASE_URL=postgres://chatops:chatops@localhost:5432/chatops_test?sslmode=disable go test ./internal/store/postgres -run 'TestMigration|TestProviderIdentity' -v`

Expected: FAIL because the second migration and domain provider values do not exist.

- [ ] **Step 3: Implement domain and SQL structures**

Add provider constants and validation. Add `ExternalIdentity` and `MessageOrigin` fields. The up migration must include:

```sql
ALTER TABLE users ALTER COLUMN feishu_open_id DROP NOT NULL;
ALTER TABLE operations ADD COLUMN message_provider text NOT NULL DEFAULT 'web';
ALTER TABLE operations ADD COLUMN message_conversation_id text NOT NULL DEFAULT '';
ALTER TABLE operations ADD COLUMN message_event_id text NOT NULL DEFAULT '';
ALTER TABLE outbox_messages ADD COLUMN provider text NOT NULL DEFAULT 'web';
CREATE TABLE system_settings (key text PRIMARY KEY, value text NOT NULL, updated_at timestamptz NOT NULL DEFAULT now());
```

Create `user_external_identities` with `UNIQUE(provider, subject_id)` and `UNIQUE(user_id, provider)`. Use a check constraint with the four accepted providers. The down migration drops dependent objects in reverse order, backfills null `users.feishu_open_id` values with `legacy-local:` plus the user UUID, and only then restores the existing `NOT NULL` constraint.

- [ ] **Step 4: Run migration and model tests**

Run: `CHATOPS_TEST_DATABASE_URL=postgres://chatops:chatops@localhost:5432/chatops_test?sslmode=disable go test ./internal/store/postgres -run 'TestMigration|TestProviderIdentity' -v`

Expected: PASS and no existing migration is reapplied.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/models.go internal/store/postgres/models.go migrations/000002_message_platforms.up.sql migrations/000002_message_platforms.down.sql internal/store/postgres/migration_test.go
git commit -m "feat: persist message provider identities"
```

### Task 3: Add store interfaces for identities and active provider

**Files:**
- Modify: `internal/store/postgres/store.go`
- Modify: `internal/store/postgres/models.go`
- Test: `internal/store/postgres/store_integration_test.go`

**Interfaces:**
- `EnsureDevelopmentAdmin(ctx context.Context) (domain.User, error)`
- `FindUserByIdentity(ctx context.Context, provider domain.MessageProvider, subject string) (domain.User, error)`
- `UpsertExternalIdentity(ctx context.Context, identity domain.ExternalIdentity) error`
- `GetActiveMessageProvider(ctx context.Context) (domain.MessageProvider, error)`
- `SetActiveMessageProvider(ctx context.Context, provider domain.MessageProvider) error`
- `ListMessageProviderIdentities(ctx context.Context, userID uuid.UUID) ([]domain.ExternalIdentity, error)`
- `HasUnfinishedMessageOperations(ctx context.Context, provider domain.MessageProvider) (bool, error)`

- [ ] **Step 1: Write failing integration tests**

```go
func TestEnsureDevelopmentAdminIsIdempotent(t *testing.T) {
    store := integrationStore(t)
    first, err := store.EnsureDevelopmentAdmin(context.Background())
    require.NoError(t, err)
    second, err := store.EnsureDevelopmentAdmin(context.Background())
    require.NoError(t, err)
    require.Equal(t, first.ID, second.ID)
}

func TestActiveProviderRoundTripsAndRejectsUnknownValue(t *testing.T) {
    store := integrationStore(t)
    require.NoError(t, store.SetActiveMessageProvider(context.Background(), domain.MessageProviderDingTalk))
    provider, err := store.GetActiveMessageProvider(context.Background())
    require.NoError(t, err)
    require.Equal(t, domain.MessageProviderDingTalk, provider)
    require.Error(t, store.SetActiveMessageProvider(context.Background(), domain.MessageProvider("telegram")))
}

func TestActiveProviderCannotSwitchWithUnfinishedMessageOperations(t *testing.T) {
    store := integrationStore(t)
    createMessageOriginOperation(t, store, domain.MessageProviderFeishu, "chat-1", "event-1")
    require.ErrorIs(t, store.SetActiveMessageProvider(context.Background(), domain.MessageProviderWeCom), domain.ErrConflict)
}
```

- [ ] **Step 2: Run and verify failure**

Run: `CHATOPS_TEST_DATABASE_URL=postgres://chatops:chatops@localhost:5432/chatops_test?sslmode=disable go test ./internal/store/postgres -run 'TestEnsureDevelopment|TestActiveProvider' -v`

Expected: FAIL because the store methods are undefined.

- [ ] **Step 3: Implement transactional store methods**

Create/update `system_settings` using `INSERT INTO system_settings(key,value) VALUES (?,?) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`. `EnsureDevelopmentAdmin` creates/finds the `web` identity with subject `local-development-admin`, creates the user with a nullable Feishu ID, finds the seeded `admin` role, and inserts `user_roles` with `ON CONFLICT DO NOTHING` in one transaction. `SetActiveMessageProvider` calls `HasUnfinishedMessageOperations` and returns `domain.ErrConflict` before switching away from a provider that owns unfinished operations. Identity lookup joins enabled users and maps record-not-found through `mapError`. Keep `FindUserByOpenID` as a Feishu compatibility wrapper.

Define `integrationStore` and the test record helper in `store_integration_test.go`:

```go
func integrationStore(t *testing.T) *Store { return New(integrationDB(t)) }

func createIntegrationApplication(t *testing.T, store *Store) domain.Application {
    t.Helper()
    app, err := store.CreateApplication(context.Background(), domain.Application{Name: "integration-app-" + uuid.NewString(), Enabled: true})
    require.NoError(t, err)
    return app
}

func createIntegrationCluster(t *testing.T, store *Store) domain.Cluster {
    t.Helper()
    cluster, err := store.CreateCluster(context.Background(), domain.Cluster{ID: uuid.New(), Name: "integration-cluster-" + uuid.NewString(), APIServer: "https://127.0.0.1", EncryptedKubeconfig: []byte("test"), CredentialVersion: 1, Enabled: true})
    require.NoError(t, err)
    return cluster
}

func createIntegrationUser(t *testing.T, store *Store) domain.User {
    t.Helper()
    user, err := store.CreateUser(context.Background(), domain.User{FeishuOpenID: "integration-" + uuid.NewString(), DisplayName: "Integration User", Enabled: true})
    require.NoError(t, err)
    return user
}

func createMessageOriginOperation(t *testing.T, store *Store, provider domain.MessageProvider, conversation, event string) {
    t.Helper()
    app := createIntegrationApplication(t, store)
    cluster := createIntegrationCluster(t, store)
    env, err := store.CreateEnvironment(context.Background(), domain.AppEnvironment{ApplicationID: app.ID, ClusterID: cluster.ID, Name: "production", Namespace: "default", Deployment: "demo", Container: "app", ImagePrefix: "registry.example/", ApprovalRequired: true})
    require.NoError(t, err)
    requester := createIntegrationUser(t, store)
    _, err = store.CreateOperation(context.Background(), domain.Operation{Kind: domain.OperationDeploy, Status: domain.StatusPendingApproval, ApplicationID: app.ID, EnvironmentID: env.ID, RequesterID: requester.ID, IdempotencyKey: event, MessageProvider: provider, ConversationID: conversation, EventID: event})
    require.NoError(t, err)
}
```

`integrationDB` creates a fresh schema per test or truncates all application tables during cleanup so randomized records never leak into a later test.

- [ ] **Step 4: Run focused integration tests**

Run: `CHATOPS_TEST_DATABASE_URL=postgres://chatops:chatops@localhost:5432/chatops_test?sslmode=disable go test ./internal/store/postgres -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/postgres/store.go internal/store/postgres/models.go internal/store/postgres/store_integration_test.go
git commit -m "feat: store active provider and external identities"
```
