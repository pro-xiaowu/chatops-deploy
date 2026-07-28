# Local Compose Deployment and Test Completion Design

Date: 2026-07-28

## 1. Context

`chatops-deploy` already contains a Go API and worker, a React operations console, PostgreSQL migrations, a production Dockerfile, Kubernetes manifests, and GitHub Actions workflows. The current `compose.yaml` starts only PostgreSQL, so the repository does not yet provide a one-command local deployment of the actual application. Automated coverage is also limited to configuration and health handler tests; the frontend has no test files and CI does not run the existing frontend test script.

This change makes the existing modular monolith runnable locally with Docker Compose, adds an explicitly development-only login path for environments without Feishu credentials, expands automated coverage around critical behavior, and publishes the result to a GitHub feature branch.

## 2. Goals

- Start PostgreSQL and the complete application with `docker compose up --build -d`.
- Serve the production React build and Go API together at `http://localhost:8080`.
- Allow local console access without Feishu credentials through an explicit development-only authentication switch.
- Reject development authentication in production configuration.
- Add meaningful backend, frontend, PostgreSQL, Kubernetes fake-client, Feishu, HTTP, and Compose smoke tests.
- Run the expanded test suite in GitHub Actions without real Feishu or Kubernetes credentials.
- Push reviewed commits to `origin/feature/local-compose-and-tests` without changing or force-pushing `origin/main`.

## 3. Non-Goals

- Connecting the automated suite to a real Feishu application.
- Creating a local Kind or Minikube cluster.
- Running deployment or rollback tests against a real Kubernetes cluster.
- Replacing Feishu OAuth or API Token authentication in production.
- Creating, merging, or force-updating a GitHub Pull Request without separate authorization.
- Publishing `chatops-deploy-conversation-export.md`; it remains local migration context.

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

## 6. Development Authentication

Development authentication reuses the production session and authorization path after login. It is not a frontend-only bypass and does not put a bearer token in browser storage.

When development authentication is enabled, startup ensures that a deterministic local administrator identity exists in PostgreSQL. The identity is marked as locally managed, enabled, and assigned the existing administrator role. Repeated startup is idempotent and does not create duplicate users or roles.

The login page obtains authentication capabilities from the backend. When development login is available, it shows a local development login action. Activating it sends a same-origin `POST /auth/dev/login` request. The handler validates the request origin, creates a normal server-side web session, sets the existing `HttpOnly` session cookie, and returns the CSRF value through the same mechanism used after OAuth login. The browser then uses the standard authenticated API and RBAC paths.

`POST /auth/dev/login` is absent in production. Tests must prove both route absence and startup rejection for invalid configuration combinations.

## 7. Docker Compose Deployment

`compose.yaml` will contain two default services:

- `postgres`: PostgreSQL 17 with its existing health check and persistent named volume.
- `app`: built from the repository Dockerfile, published on host port `8080`, and started only after PostgreSQL is healthy.

The application service uses explicit local-only defaults for the database URL, a valid 32-byte base64 kubeconfig master key, the bootstrap administrator token, `CHATOPS_RUNTIME_ENV=development`, `CHATOPS_DEV_AUTH_ENABLED=true`, `CHATOPS_SECURITY_PUBLIC_BASE_URL=http://localhost:8080`, and insecure cookies for local HTTP. These values are documented as development credentials and must not be reused for an internet-facing deployment.

The application binary will support an internal readiness probe command so the distroless image can be health-checked without adding a shell, curl, or wget. Compose uses that command to query the application's local `/readyz` endpoint. The service becomes healthy only after migrations complete and PostgreSQL readiness succeeds.

The expected local workflow is:

```bash
docker compose up --build -d
docker compose ps
docker compose logs app
docker compose down
```

Resetting local state is an explicit destructive operation documented separately with `docker compose down --volumes`. Normal stop commands do not remove the PostgreSQL volume.

## 8. Error Handling and Security Boundaries

- Invalid runtime/development-auth combinations stop the process with a stable configuration error.
- Failed local administrator provisioning stops startup instead of leaving an unusable login button.
- Development login failures return the existing stable JSON error envelope and do not reveal database details.
- The capability endpoint is public but returns only supported login methods.
- Session cookies remain `HttpOnly` and `SameSite=Lax`; local Compose sets `Secure=false` only because it serves plain HTTP on localhost.
- Authenticated writes still require CSRF protection, and development users pass through the same RBAC checks as OAuth users.
- No fake Kubernetes success response is added. Operations requiring an unconfigured or unreachable cluster fail through the existing Kubernetes error path.
- Test logs and CI artifacts must not contain session IDs, bootstrap tokens, kubeconfig contents, Feishu secrets, or the master key.

## 9. Automated Test Design

### 9.1 Go Unit and HTTP Tests

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
- stable HTTP errors that do not leak sensitive dependency failures.

Tests should assert externally observable behavior and use real package code. Mocks are limited to external boundaries such as the database repository interface, HTTP servers, clock, and Kubernetes client.

### 9.2 PostgreSQL Integration Tests

Use PostgreSQL 17 in CI and a dedicated test database URL. Integration tests cover:

- applying all embedded migrations to an empty database;
- bootstrap administrator and development administrator idempotency;
- operation and approval transaction behavior;
- unique idempotency and inbox event constraints;
- outbox persistence and retry selection;
- queued operation claiming and database locking behavior.

Each test owns isolated data and performs deterministic cleanup. Integration tests skip with an explicit message only when the dedicated test database variable is absent during an ordinary developer unit-test run; CI always supplies it.

### 9.3 Kubernetes Tests

Use `client-go` fake clients and reactors to cover:

- updating only the registered container image;
- rejecting a missing container and disallowed image prefix;
- successful and failed rollout condition evaluation;
- selecting a ReplicaSet revision and applying a cleaned Pod template during rollback;
- conflict retry behavior without contacting a cluster.

No Kind, Minikube, kubeconfig, or network access is required.

### 9.4 Frontend Tests

Add Vitest, Testing Library, `jest-dom`, and request mocking suitable for the existing React stack. Cover:

- capability-driven display of development login versus Feishu login;
- successful development login navigation;
- authenticated and unauthorized route behavior;
- representative dashboard/list loading, empty, and error states;
- approval action confirmation and server error display;
- CSRF header attachment for browser write requests.

Frontend tests run headlessly and do not depend on the Go process.

### 9.5 Compose and Browser Smoke Test

The smoke workflow builds and starts the Compose stack, waits for both services to become healthy, and verifies:

1. `/healthz` and `/readyz` return success.
2. The SPA entry point and a built asset are served by the Go container.
3. Development login creates a session.
4. An authenticated API request succeeds.
5. A Playwright browser can log in and render the operations console without overlapping or blank primary content at desktop and mobile viewport sizes.

The workflow captures application logs and browser screenshots on failure, then always tears down containers. It does not remove a developer's normal local volume; CI uses its own ephemeral runner.

## 10. CI Changes

Update `.github/workflows/ci.yml` so feature branches and Pull Requests run:

- Go formatting and module consistency checks;
- `go vet` and `go test ./... -race`;
- PostgreSQL integration tests against a PostgreSQL 17 service container;
- frontend `npm ci`, type checking, linting, unit tests, and production build;
- Docker image build;
- Compose and Playwright smoke verification after the backend and frontend jobs succeed.

The backend and frontend jobs produce coverage reports as artifacts. This change does not introduce a repository-wide numeric coverage gate because the current baseline is largely untested; new development-auth and local-deployment behavior must nevertheless have direct behavior tests and all CI jobs must pass.

## 11. Documentation and Repository Hygiene

Update the local-running documentation with Linux/macOS and PowerShell commands for startup, status, logs, login, stop, and explicit volume reset. Document the local URL and the fact that default Compose credentials are development-only.

Add `chatops-deploy-conversation-export.md` to the repository ignore rules so migration context cannot be included by a broad `git add`. Do not include real `.env` files, kubeconfigs, generated secrets, Playwright failure artifacts, coverage outputs, or local database data.

The existing `LICENSE` line-ending-only change is not part of this work. README content changes are reviewed separately from unrelated line-ending differences.

## 12. GitHub Delivery

Work is performed on `feature/local-compose-and-tests`, created from the current `main`. Commits are grouped by independently reviewable behavior: runtime/Compose support, development authentication, backend integration tests, frontend/browser tests, and CI/documentation.

Before pushing, run the full local verification that the available environment permits, including the Compose stack. Push with a normal upstream-tracking push to `origin/feature/local-compose-and-tests`. Do not force-push and do not update `origin/main` directly.

After pushing, verify the remote branch commit. Provide the GitHub compare or Pull Request URL, but do not create or merge a Pull Request unless authenticated tooling is available and the user separately authorizes that external action.

## 13. Acceptance Criteria

1. `docker compose up --build -d` starts PostgreSQL and the complete application.
2. Both Compose services become healthy and `http://localhost:8080/readyz` succeeds.
3. The React console is served from the application container.
4. A local administrator can use development login and access authorized console APIs.
5. Development authentication is absent and rejected by configuration in production.
6. The expanded Go, PostgreSQL, Kubernetes fake-client, Feishu, HTTP, React, and Compose smoke tests pass.
7. GitHub Actions executes the expanded backend, frontend, integration, image, and smoke checks.
8. No real secret or conversation export is committed.
9. The completed commits are pushed to `origin/feature/local-compose-and-tests` without modifying `origin/main`.
