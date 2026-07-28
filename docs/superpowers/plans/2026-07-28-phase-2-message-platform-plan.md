# Message Platform and API Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (recommended for this phase) or superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Normalize inbound commands and outbound notifications, implement Web-only/Feishu/WeCom/DingTalk provider adapters, add provider selection APIs, and add development-only Session login using the formal RBAC path.

**Architecture:** `internal/messaging` owns provider-neutral command, callback, notification, and registry interfaces. Each adapter owns protocol verification, payload mapping, and HTTP client behavior. The HTTP handler delegates normalized events to `application.Service`; it never branches on provider-specific JSON after decoding.

**Tech Stack:** Go 1.25, Gin, `httptest`, Feishu HTTP API, WeCom callback/application-message API, DingTalk event/robot API, GORM store interfaces, Argon2id Session service.

## Global Constraints

- Provider secrets are read from `config.Config` only and never returned by API responses.
- Exactly one provider is active; `web` has no third-party webhook or outbound call.
- Feishu OAuth remains a browser login option only when configured; WeCom/DingTalk adapters do not implement browser OAuth.
- Every adapter test uses local `httptest.Server` or pure payload fixtures; no provider network calls run in CI.
- Development login is registered only when runtime environment is `development` and the explicit flag is true.

### Task 1: Define the normalized messaging contract and Web-only provider

**Files:**
- Create: `internal/messaging/message.go`
- Create: `internal/messaging/registry.go`
- Create: `internal/adapter/messaging/web.go`
- Test: `internal/messaging/registry_test.go`
- Test: `internal/adapter/messaging/web_test.go`

**Interfaces:**

```go
type Provider interface {
    Name() domain.MessageProvider
    Capabilities(context.Context) Capabilities
    Decode(context.Context, Request) (Incoming, error)
    Send(context.Context, Notification) error
    Check(context.Context) error
}

type Registry interface {
    Active(context.Context) (Provider, error)
    Select(context.Context, domain.MessageProvider) error
    Capabilities(context.Context) []Capabilities
}

type ProviderSettingsStore interface {
    GetActiveMessageProvider(context.Context) (domain.MessageProvider, error)
    SetActiveMessageProvider(context.Context, domain.MessageProvider) error
    HasUnfinishedMessageOperations(context.Context, domain.MessageProvider) (bool, error)
}
```

`Incoming` contains provider, event ID, conversation ID, subject ID, display name, normalized command or approval callback, and challenge response. `Notification` contains operation ID, destination, text, and optional approval card data.

- [ ] **Step 1: Write failing tests**

```go
func TestWebProviderDoesNotDecodeOrSendExternalMessages(t *testing.T) {
    provider := messagingweb.New()
    require.True(t, provider.Capabilities(context.Background()).Configured)
    require.EqualError(t, provider.Send(context.Background(), messaging.Notification{Text: "ignored"}), "web provider does not send messages")
}

func TestRegistryRejectsProviderWithoutConfiguration(t *testing.T) {
    registry := messaging.NewRegistry(storeStub{active: domain.MessageProviderWeb}, []messaging.Provider{messagingweb.New(), fakeProvider{name: domain.MessageProviderFeishu, configured: false}})
    require.EqualError(t, registry.Select(context.Background(), domain.MessageProviderFeishu), "message provider feishu is not configured")
}

type fakeProvider struct { name domain.MessageProvider; configured bool }
func (p fakeProvider) Name() domain.MessageProvider { return p.name }
func (p fakeProvider) Capabilities(context.Context) messaging.Capabilities { return messaging.Capabilities{Provider: p.name, Configured: p.configured} }
func (p fakeProvider) Decode(context.Context, messaging.Request) (messaging.Incoming, error) { return messaging.Incoming{}, errors.New("not implemented") }
func (p fakeProvider) Send(context.Context, messaging.Notification) error { return nil }
func (p fakeProvider) Check(context.Context) error { if !p.configured { return errors.New("not configured") }; return nil }

type storeStub struct { active domain.MessageProvider }
func (s storeStub) GetActiveMessageProvider(context.Context) (domain.MessageProvider, error) { return s.active, nil }
func (s storeStub) SetActiveMessageProvider(_ context.Context, provider domain.MessageProvider) error { s.active = provider; return nil }
func (s storeStub) HasUnfinishedMessageOperations(context.Context, domain.MessageProvider) (bool, error) { return false, nil }
```

- [ ] **Step 2: Run and verify failure**

Run: `go test ./internal/messaging ./internal/adapter/messaging -v`

Expected: FAIL because the normalized types, registry, and Web-only adapter do not exist.

- [ ] **Step 3: Implement minimal types, registry, and Web-only behavior**

Implement provider capability status (`Configured`, `Healthy`, `Reason`), registry selection with `GetActiveMessageProvider`/`SetActiveMessageProvider`, and Web-only `Check` returning nil. `Decode` and `Send` return stable unsupported errors. Preserve the persisted provider if a requested provider is unavailable.

- [ ] **Step 4: Run focused tests**

Run: `go test ./internal/messaging ./internal/adapter/messaging -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/messaging internal/adapter/messaging
git commit -m "feat: define normalized message provider contract"
```

### Task 2: Adapt Feishu to the normalized contract

**Files:**
- Modify: `internal/adapter/feishu/client.go`
- Modify: `internal/adapter/feishu/webhook.go`
- Create: `internal/adapter/feishu/provider.go`
- Test: `internal/adapter/feishu/provider_test.go`

**Interfaces:**
- `feishu.NewProvider(config.FeishuConfig) messaging.Provider`
- Existing `VerifySignature`, `DecryptEvent`, `DecodeEvent`, `Command`, `ApprovalCard`, and OAuth methods remain available.

- [ ] **Step 1: Add failing provider contract tests**

Test a valid signed event normalizes to `ProviderFeishu`, event ID, chat ID, sender OpenID, and `/deploy` command. Test an approval card normalizes operation ID and decision. Test `Send` uses the existing token endpoint and returns a stable error on non-2xx without exposing the response body.

- [ ] **Step 2: Run and observe failure**

Run: `go test ./internal/adapter/feishu -run TestProvider -v`

Expected: FAIL because `NewProvider` and normalized `Decode`/`Send` methods are missing.

- [ ] **Step 3: Implement the adapter as a thin wrapper**

Keep provider-specific parsing and card rendering inside the adapter. Prefix normalized inbox IDs with `feishu:` before returning them to the application layer and redact response bodies from returned errors.

- [ ] **Step 4: Run all Feishu tests**

Run: `go test ./internal/adapter/feishu -v`

Expected: PASS, including existing signature/decryption behavior.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/feishu
git commit -m "refactor: adapt Feishu to message provider contract"
```

### Task 3: Implement WeCom and DingTalk adapters

**Files:**
- Create: `internal/adapter/wecom/provider.go`
- Create: `internal/adapter/wecom/provider_test.go`
- Create: `internal/adapter/dingtalk/provider.go`
- Create: `internal/adapter/dingtalk/provider_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `config/config.example.yaml`
- Modify: `.env.example`

**Interfaces:**
- `wecom.NewProvider(config.WeComConfig) messaging.Provider`
- `dingtalk.NewProvider(config.DingTalkConfig) messaging.Provider`

- [ ] **Step 1: Write failing `httptest` fixtures**

For each adapter, add tests for callback verification, a normalized deploy command, an approval action, an outbound notification request, a non-2xx response, and missing required credentials. Assert request method, path, authentication headers, and decoded body without asserting private helper calls.

- [ ] **Step 2: Run focused tests and observe missing adapters**

Run: `go test ./internal/adapter/wecom ./internal/adapter/dingtalk -v`

Expected: FAIL because the packages and constructors do not exist.

- [ ] **Step 3: Implement protocol-specific configuration and redacted HTTP clients**

Add `WeComConfig` fields `CorpID`, `AgentID`, `Secret`, `Token`, `EncodingAESKey`, and `APIBaseURL`. Add `DingTalkConfig` fields `ClientID`, `ClientSecret`, `RobotCode`, `EventToken`, `EventAESKey`, and `APIBaseURL`. Bind their `CHATOPS_WECOM_*` and `CHATOPS_DINGTALK_*` variables. A provider reports `Configured=true` only when all fields required by its chosen callback/message path are non-empty. Keep API paths and signatures inside each adapter and return stable provider errors without response secrets.

- [ ] **Step 4: Run adapter and configuration tests**

Run: `go test ./internal/adapter/wecom ./internal/adapter/dingtalk ./internal/messaging ./internal/config -v`

Expected: PASS with no external network access.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/wecom internal/adapter/dingtalk internal/config/config.go internal/config/config_test.go config/config.example.yaml .env.example
git commit -m "feat: add WeCom and DingTalk message adapters"
```

### Task 4: Add provider-aware application, webhook, and settings API

**Files:**
- Modify: `internal/application/service.go`
- Modify: `internal/transport/http/handler/api.go`
- Modify: `internal/transport/http/router.go`
- Create: `internal/transport/http/handler/auth_test.go`
- Create: `internal/transport/http/handler/provider_test.go`

**Interfaces:**
- `GET /auth/capabilities` returns configured login and message-provider booleans only.
- `POST /auth/dev/login` creates a normal Session only in development with the explicit flag.
- `GET /api/v1/me` returns the authenticated local user and is the frontend Session probe.
- `POST /webhooks/:provider` rejects inactive providers and Web-only mode with 404.
- `GET /api/v1/settings/message-provider` returns all capabilities and the active provider.
- `PUT /api/v1/settings/message-provider` accepts `{ "provider": "web|feishu|wecom|dingtalk" }` and requires admin permission.
- `POST /api/v1/settings/message-provider/:provider/check` performs a health check without switching.
- `GET /api/v1/users/:id/identities` and `POST /api/v1/users/:id/identities` list and add provider-specific external identities for administrator-managed user mapping.

- [ ] **Step 1: Write failing handler tests**

```go
func TestDevLoginIsUnavailableInProduction(t *testing.T) {
    router := testRouter(t, config.Config{RuntimeEnvironment: "production", DevAuthEnabled: false})
    response := perform(router, http.MethodPost, "/auth/dev/login", `{"return_path":"/"}`)
    require.Equal(t, http.StatusNotFound, response.Code)
}

func TestAdminCanSelectConfiguredProvider(t *testing.T) {
    router, admin := authenticatedAdminRouter(t)
    response := performWithSession(router, admin, http.MethodPut, "/api/v1/settings/message-provider", `{"provider":"dingtalk"}`)
    require.Equal(t, http.StatusOK, response.Code)
    require.JSONEq(t, `{"data":{"provider":"dingtalk"}}`, response.Body.String())
}
```

- [ ] **Step 2: Run handler tests and observe failure**

Run: `go test ./internal/transport/http/handler -run 'TestDevLogin|TestAdminCanSelect' -v`

Expected: FAIL because the routes and provider-aware API dependencies do not exist.

- [ ] **Step 3: Implement route registration and application delegation**

Make `NewAPI` receive runtime flags, the messaging registry, and the identity store. Add a single Session-cookie helper for OAuth and development login. Development login calls `EnsureDevelopmentAdmin` and `CreateSession`. Add `/api/v1/me` for Session probing and identity management routes for administrators. Generic webhook handling decodes through the active provider, resolves `ExternalIdentity`, persists a provider-prefixed inbox event before creating an operation, and sends the normalized response through the originating provider. Settings endpoints require `ActionAdmin` and preserve the previous provider if `Check` fails or unfinished operations prevent switching.

- [ ] **Step 4: Run focused and existing HTTP tests**

Run: `go test ./internal/transport/http/... -v`

Expected: PASS, including health and stable error tests.

- [ ] **Step 5: Commit**

```bash
git add internal/application/service.go internal/transport/http/handler/api.go internal/transport/http/handler/auth_test.go internal/transport/http/handler/provider_test.go internal/transport/http/router.go
git commit -m "feat: add development login and provider settings API"
```

### Task 5: Wire provider registry and origin-aware worker notifications

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `internal/worker/worker.go`
- Modify: `internal/store/postgres/store.go`
- Test: `cmd/server/main_test.go`
- Test: `internal/worker/worker_test.go`

**Interfaces:**
- `run` constructs all four providers and seeds the configured active provider without external calls for `web`.
- Worker notification dispatch uses an operation's persisted provider and destination, not the current provider.

- [ ] **Step 1: Write failing wiring tests**

Test Web-only startup with empty third-party secrets. Test that a configured-but-unhealthy provider does not replace the persisted active provider. Test that a succeeded operation with Feishu origin is sent through a Feishu spy after the active provider changes to WeCom.

- [ ] **Step 2: Run focused tests and observe missing dependencies**

Run: `go test ./cmd/server ./internal/worker -run 'Test.*Provider|Test.*Origin' -v`

Expected: FAIL because `run` and `Worker` do not accept a registry or notification sender.

- [ ] **Step 3: Implement dependency assembly and origin-aware dispatch**

Construct all providers from config, build the registry, call `EnsureDevelopmentAdmin` only when development auth is enabled, and pass the registry to API and Worker. Add provider/destination metadata to outbox messages and select the provider by persisted origin for approval/result notifications. Optional provider credentials must not block startup when the active provider is `web`.

- [ ] **Step 4: Run backend regression tests**

Run: `go test ./cmd/server ./internal/worker ./internal/application ./internal/store/postgres -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/server/main.go cmd/server/main_test.go internal/worker/worker.go internal/worker/worker_test.go internal/store/postgres/store.go
git commit -m "feat: wire provider registry and origin-aware notifications"
```
