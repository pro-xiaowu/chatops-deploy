# Local Compose Deployment and Test Completion Design

Date: 2026-07-28

## 1. Context

`chatops-deploy` already contains a Go API and worker, a React operations console, PostgreSQL migrations, a production Dockerfile, Kubernetes manifests, and GitHub Actions workflows. The current `compose.yaml` starts only PostgreSQL, so the repository does not yet provide a one-command local deployment of the actual application. Automated coverage is also limited to configuration and health handler tests; the frontend has no test files and CI does not run the existing frontend test script.

This change makes the existing modular monolith runnable locally with Docker Compose, adds an explicitly development-only login path, lets an administrator choose Feishu, WeCom, DingTalk, or Web-only message delivery, expands automated coverage around critical behavior, and publishes the result to a GitHub feature branch.

The supported local runtime is Linux. Docker Compose, shell scripts, and CI verification are written for a Linux environment.

## 2. Goals

- Start PostgreSQL and the complete application with `docker compose up --build -d`.
- Serve the production React build and Go API together at `http://localhost:8080`.
- Allow local console access without Feishu credentials through an explicit development-only authentication switch.
- Reject development authentication in production configuration.
- Let an administrator select one active message platform from Feishu, WeCom, DingTalk, or Web-only mode.
- Keep deployment, approval, audit, and worker behavior independent of the selected message platform.
- Add meaningful backend, frontend, PostgreSQL, Kubernetes fake-client, Feishu, HTTP, and Compose smoke tests.
- Run the expanded test suite in GitHub Actions without real Feishu or Kubernetes credentials.
- Push reviewed commits to `origin/feature/local-compose-and-tests` without changing or force-pushing `origin/main`.
- Document Linux as the supported runtime for local deployment and verification.

## 3. Non-Goals

- Connecting the automated suite to a real Feishu application.
- Creating a local Kind or Minikube cluster.
- Running deployment or rollback tests against a real Kubernetes cluster.
- Replacing Feishu OAuth or API Token authentication in production.
- Adding WeCom or DingTalk as browser OAuth providers; this change affects ChatOps commands, approval callbacks, and notifications only.
- Broadcasting the same command or notification through multiple active message platforms at once.
- Creating, merging, or force-updating a GitHub Pull Request without separate authorization.
- Publishing `chatops-deploy-conversation-export.md`; it remains local migration context.
- Supporting Windows as a native runtime or providing PowerShell-specific deployment instructions.

## 4. Chosen Approach

Extend the existing single-image architecture. Docker Compose will run PostgreSQL and the same application image used for production-style deployment. The Go process remains responsible for the API, worker, authentication, migrations, and serving the compiled React assets.

This is preferred over separate frontend and backend development containers because it tests the repository's intended same-origin delivery model. It is preferred over a mock backend because the local deployment must exercise real migrations, sessions, RBAC, HTTP routing, and application startup.

## 5. Runtime Configuration

Add a runtime environment setting with exactly two accepted values:

- `production`: the default when no value is supplied.
- `development`: required for local development authentication.

The environment variable is `CHATOPS_RUNTIME_ENV`. Add `CHATOPS_DEV_AUTH_ENABLED`, which defaults to `false`.

Configuration validation follows these rules:

1. Unknown runtime environment values fail startup.
2. `CHATOPS_DEV_AUTH_ENABLED=true` with `CHATOPS_RUNTIME_ENV=production` fails startup.
3. The development login route is not registered unless the runtime environment is `development` and the switch is `true`.
4. Existing database URL and kubeconfig master-key validation remains mandatory in every environment.
5. Empty Feishu credentials do not block local startup, but Feishu OAuth and webhook capabilities report unavailable and are not presented as usable login paths.

The public capability response used by the login page will expose only non-sensitive booleans such as whether Feishu login and development login are available. It must not expose credentials, tokens, key fingerprints, or internal configuration values.

Add `CHATOPS_MESSAGE_PROVIDER` as the bootstrap message-platform selection. Accepted values are `web`, `feishu`, `wecom`, and `dingtalk`; the default is `web`. The environment value seeds the database only when no active provider setting exists. After bootstrap, an administrator can change the active provider from the system settings API and console without changing the deployment manifest.

## 6. Development Authentication

Development authentication reuses the production session and authorization path after login. It is not a frontend-only bypass and does not put a bearer token in browser storage.

When development authentication is enabled, startup ensures that a deterministic local administrator identity exists in PostgreSQL. The identity is marked as locally managed, enabled, and assigned the existing administrator role. Repeated startup is idempotent and does not create duplicate users or roles.

The login page obtains authentication capabilities from the backend. When development login is available, it shows a local development login action. Activating it sends a same-origin `POST /auth/dev/login` request. The handler validates the request origin, creates a normal server-side web session, sets the existing `HttpOnly` session cookie, and returns the CSRF value through the same mechanism used after OAuth login. The browser then uses the standard authenticated API and RBAC paths.

`POST /auth/dev/login` is absent in production. Tests must prove both route absence and startup rejection for invalid configuration combinations.

Browser authentication and ChatOps message delivery remain separate concerns. Selecting WeCom, DingTalk, or Web-only messaging does not silently turn that provider into a browser OAuth provider. Production browser login continues to use the existing Feishu OAuth capability when configured; local Compose uses development login.

## 7. Message Platform Selection

Introduce a provider-neutral message contract between the application layer and platform adapters. The contract covers:

- provider identity and configuration health;
- webhook verification and event parsing;
- command normalization for deploy, rollback, status, and help;
- approval callback normalization;
- approval request, status, and result notification delivery;
- stable retry classification and provider-safe error summaries.

The provider registry contains four adapters:

- `feishu`: the existing Feishu event, card, OAuth-independent messaging, and notification behavior.
- `wecom`: enterprise WeChat callback verification, command events, template-card approval actions, and application messages.
- `dingtalk`: DingTalk event verification, robot commands, interactive approval actions, and result messages.
- `web`: a no-op message adapter. It registers no message webhook and sends no external notification; all operations and approvals remain available through the Web console.

Only one provider is active per service instance. Provider credentials remain environment or mounted-secret values and are never written through the browser. The system settings page lists all four options but enables selection only for `web` or a provider whose required credentials are present and whose adapter health check succeeds. The API repeats this validation before persisting a switch.

The active provider is stored in a small versioned system setting so all API and worker replicas use the same choice. Switching affects new inbound events and outbound notifications only. Existing operations retain their originating provider and conversation identifiers so an in-flight approval or result is routed back through the provider that created it. A provider cannot be disabled while it owns unfinished message-originated operations unless an administrator explicitly resolves or cancels them.

Provider-specific external user identifiers move behind a normalized identity mapping keyed by provider and external subject ID. A single local user may be linked to Feishu, WeCom, and DingTalk identities without duplicating RBAC assignments. Incoming events with an unknown or disabled identity are rejected before an operation is created.

Application services, the PostgreSQL operation state machine, Kubernetes execution, RBAC, and audit logic consume normalized commands and notification requests. They do not branch on Feishu, WeCom, or DingTalk protocol fields.

## 8. Docker Compose Deployment

`compose.yaml` will contain two default services:

- `postgres`: PostgreSQL 17 with its existing health check and persistent named volume.
- `app`: built from the repository Dockerfile, published on host port `8080`, and started only after PostgreSQL is healthy.

The application service uses explicit local-only defaults for the database URL, a valid 32-byte base64 kubeconfig master key, the bootstrap administrator token, `CHATOPS_RUNTIME_ENV=development`, `CHATOPS_DEV_AUTH_ENABLED=true`, `CHATOPS_MESSAGE_PROVIDER=web`, `CHATOPS_SECURITY_PUBLIC_BASE_URL=http://localhost:8080`, and insecure cookies for local HTTP. These values are documented as development credentials and must not be reused for an internet-facing deployment.

The application binary will support an internal readiness probe command so the distroless image can be health-checked without adding a shell, curl, or wget. Compose uses that command to query the application's local `/readyz` endpoint. The service becomes healthy only after migrations complete and PostgreSQL readiness succeeds.

The expected local workflow is:

```bash
docker compose up --build -d
docker compose ps
docker compose logs app
docker compose down
```

Resetting local state is an explicit destructive operation documented separately with `docker compose down --volumes`. Normal stop commands do not remove the PostgreSQL volume.

## 9. Error Handling and Security Boundaries

- Invalid runtime/development-auth combinations stop the process with a stable configuration error.
- Failed local administrator provisioning stops startup instead of leaving an unusable login button.
- Development login failures return the existing stable JSON error envelope and do not reveal database details.
- The capability endpoint is public but returns only supported login methods.
- Session cookies remain `HttpOnly` and `SameSite=Lax`; local Compose sets `Secure=false` only because it serves plain HTTP on localhost.
- Authenticated writes still require CSRF protection, and development users pass through the same RBAC checks as OAuth users.
- Selecting an unconfigured or unhealthy message provider is rejected without changing the active provider.
- Provider webhook errors use stable public responses and never include signing secrets, access tokens, encrypted payload keys, or raw credential values.
- Provider switching does not reroute an existing operation to a different external conversation.
- No fake Kubernetes success response is added. Operations requiring an unconfigured or unreachable cluster fail through the existing Kubernetes error path.
- Test logs and CI artifacts must not contain session IDs, bootstrap tokens, kubeconfig contents, Feishu secrets, or the master key.

## 10. Automated Test Design

### 10.1 Go Unit and HTTP Tests

Add focused tests for:

- runtime environment defaults and accepted values;
- production rejection of development authentication;
- development login route registration and production route absence;
- idempotent provisioning of the local administrator identity;
- session cookie creation, CSRF behavior, and authenticated principal resolution;
- API Token authentication and representative RBAC allow/deny cases;
- production self-approval rejection, direct queuing for non-production environments, invalid state transitions, duplicate idempotency keys, and invalid image prefixes;
- kubeconfig encryption round trips and invalid-key failures;
- Feishu signature verification, encrypted event handling, command parsing, duplicate event handling, and approval callbacks;
- provider registry selection, Web-only behavior, unavailable-provider rejection, and provider switching rules;
- WeCom and DingTalk verification, command normalization, approval action parsing, outbound notification requests, and sensitive error redaction;
- external identity mapping across multiple providers without duplicating local RBAC assignments;
- stable HTTP errors that do not leak sensitive dependency failures.

Tests should assert externally observable behavior and use real package code. Mocks are limited to external boundaries such as the database repository interface, HTTP servers, clock, and Kubernetes client.

### 10.2 PostgreSQL Integration Tests

Use PostgreSQL 17 in CI and a dedicated test database URL. Integration tests cover:

- applying all embedded migrations to an empty database;
- bootstrap administrator and development administrator idempotency;
- active-provider setting persistence and provider-specific external identity uniqueness;
- operation and approval transaction behavior;
- unique idempotency and inbox event constraints;
- outbox persistence and retry selection;
- queued operation claiming and database locking behavior.

Each test owns isolated data and performs deterministic cleanup. Integration tests skip with an explicit message only when the dedicated test database variable is absent during an ordinary developer unit-test run; CI always supplies it.

### 10.3 Kubernetes Tests

Use `client-go` fake clients and reactors to cover:

- updating only the registered container image;
- rejecting a missing container and disallowed image prefix;
- successful and failed rollout condition evaluation;
- selecting a ReplicaSet revision and applying a cleaned Pod template during rollback;
- conflict retry behavior without contacting a cluster.

No Kind, Minikube, kubeconfig, or network access is required.

### 10.4 Frontend Tests

Add Vitest, Testing Library, `jest-dom`, and request mocking suitable for the existing React stack. Cover:

- capability-driven display of development login versus Feishu login;
- system-setting options for Feishu, WeCom, DingTalk, and Web-only mode;
- disabled states and explanations for providers without valid credentials;
- successful provider selection and server-side rejection without losing the previous selection;
- successful development login navigation;
- authenticated and unauthorized route behavior;
- representative dashboard/list loading, empty, and error states;
- approval action confirmation and server error display;
- CSRF header attachment for browser write requests.

Frontend tests run headlessly and do not depend on the Go process.

### 10.5 Compose and Browser Smoke Test

The smoke workflow builds and starts the Compose stack, waits for both services to become healthy, and verifies:

1. `/healthz` and `/readyz` return success.
2. The SPA entry point and a built asset are served by the Go container.
3. Development login creates a session.
4. An authenticated API request succeeds.
5. Web-only mode is active and no third-party message webhook is exposed as usable.
6. A Playwright browser can log in, view all four provider choices, and render the operations console without overlapping or blank primary content at desktop and mobile viewport sizes.

The workflow captures application logs and browser screenshots on failure, then always tears down containers. It does not remove a developer's normal local volume; CI uses its own ephemeral runner.

## 11. CI Changes

Update `.github/workflows/ci.yml` so feature branches and Pull Requests run:

- Go formatting and module consistency checks;
- `go vet` and `go test ./... -race`;
- PostgreSQL integration tests against a PostgreSQL 17 service container;
- frontend `npm ci`, type checking, linting, unit tests, and production build;
- Docker image build;
- Compose and Playwright smoke verification after the backend and frontend jobs succeed.

The backend and frontend jobs produce coverage reports as artifacts. This change does not introduce a repository-wide numeric coverage gate because the current baseline is largely untested; new development-auth and local-deployment behavior must nevertheless have direct behavior tests and all CI jobs must pass.

## 12. Documentation and Repository Hygiene

Update the local-running documentation with Linux shell commands for startup, status, logs, login, stop, and explicit volume reset. Document the local URL and the fact that default Compose credentials are development-only.

Document the four message-platform modes, required secret groups, the difference between browser authentication and message delivery, provider health status, safe switching behavior, and Web-only local operation. Documentation must not include real provider secrets.

Add `chatops-deploy-conversation-export.md` to the repository ignore rules so migration context cannot be included by a broad `git add`. Do not include real `.env` files, kubeconfigs, generated secrets, Playwright failure artifacts, coverage outputs, or local database data.

The existing `LICENSE` line-ending-only change is not part of this work. README content changes are reviewed separately from unrelated line-ending differences.

## 13. GitHub Delivery

Work is performed on `feature/local-compose-and-tests`, created from the current `main`. Commits are grouped by independently reviewable behavior: runtime/Compose support, development authentication, backend integration tests, frontend/browser tests, and CI/documentation.

Before pushing, run the full local verification that the available environment permits, including the Compose stack. Push with a normal upstream-tracking push to `origin/feature/local-compose-and-tests`. Do not force-push and do not update `origin/main` directly.

After pushing, verify the remote branch commit. Provide the GitHub compare or Pull Request URL, but do not create or merge a Pull Request unless authenticated tooling is available and the user separately authorizes that external action.

## 14. Acceptance Criteria

1. `docker compose up --build -d` starts PostgreSQL and the complete application.
2. Both Compose services become healthy and `http://localhost:8080/readyz` succeeds.
3. The React console is served from the application container.
4. A local administrator can use development login and access authorized console APIs.
5. Development authentication is absent and rejected by configuration in production.
6. The system settings page offers Feishu, WeCom, DingTalk, and Web-only messaging, and rejects unavailable provider selections.
7. Web-only mode requires no third-party credentials and keeps Web deployment, rollback, approval, and audit workflows usable.
8. Message-originated operations keep their originating provider routing across an active-provider switch.
9. The expanded Go, PostgreSQL, Kubernetes fake-client, Feishu, WeCom, DingTalk, HTTP, React, and Compose smoke tests pass.
10. GitHub Actions executes the expanded backend, frontend, integration, image, and smoke checks.
11. No real secret or conversation export is committed.
12. The completed commits are pushed to `origin/feature/local-compose-and-tests` without modifying `origin/main`.
13. Local deployment and CI instructions clearly require Linux; Windows is outside the supported runtime scope.
