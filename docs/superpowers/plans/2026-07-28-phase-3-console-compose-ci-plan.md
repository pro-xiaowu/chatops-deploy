# Console, Compose, Tests, and Delivery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (recommended for this phase) or superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the React console authenticate through development Session or Feishu OAuth, expose message-provider settings, add backend/frontend/integration/browser tests, make Docker Compose run the app, and publish the verified feature branch.

**Architecture:** The Go container serves the compiled SPA and all API routes on one origin. React asks the public capabilities endpoint for available login/provider options, keeps no browser bearer token for development login, and calls provider settings through the existing CSRF-aware API client. CI runs independent Go/PostgreSQL, frontend, image, and Compose smoke jobs.

**Tech Stack:** React 19, TypeScript, Ant Design, TanStack Query, Vitest, Testing Library, MSW, Playwright, Docker Compose, GitHub Actions, PostgreSQL 17.

## Global Constraints

- Use the real Session cookie and CSRF flow for development login; never add a hard-coded API token to frontend code.
- Web-only Compose must work with no Feishu, WeCom, or DingTalk credentials.
- Browser smoke tests capture desktop and mobile screenshots and assert the main content is visible without layout overlap.
- CI runs frontend tests and PostgreSQL integration tests; image builds do not push to a registry.
- Do not stage `chatops-deploy-conversation-export.md`, `.env`, generated assets, or user-owned line-ending-only changes.

### Task 1: Add frontend test harness and public authentication capability client

**Files:**
- Modify: `web/package.json`
- Modify: `web/package-lock.json`
- Create: `web/vitest.config.ts`
- Create: `web/src/test/setup.ts`
- Modify: `web/src/api.ts`
- Modify: `web/src/types.ts`
- Create: `web/src/api.test.ts`

**Interfaces:**
- `api.capabilities(): Promise<Capabilities>` calls `/auth/capabilities` without API Token authentication.
- `api.me(): Promise<User>` calls `/api/v1/me` to determine whether the Session is authenticated.
- `api.devLogin(): Promise<void>` posts `/auth/dev/login` with same-origin credentials.
- `api.messageProvider(): Promise<MessageProviderSettings>` and `api.selectMessageProvider(provider)` use authenticated CSRF-aware requests.

- [ ] **Step 1: Add failing API/client tests**

```ts
it('posts development login with credentials and no bearer token', async () => {
  const request = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({data: {}}), {status: 200}))
  await api.devLogin()
  expect(request).toHaveBeenCalledWith('/auth/dev/login', expect.objectContaining({method: 'POST', credentials: 'include'}))
  expect(JSON.stringify(request.mock.calls[0][1]?.headers)).not.toContain('Bearer')
})

it('returns all provider options from capabilities', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({data: {dev_login: true, feishu_login: false, message_providers: [{provider: 'web', selectable: true}, {provider: 'feishu', selectable: false}, {provider: 'wecom', selectable: false}, {provider: 'dingtalk', selectable: false}]}}), {status: 200}))
  await expect(api.capabilities()).resolves.toMatchObject({dev_login: true, message_providers: expect.arrayContaining([{provider: 'web'}])})
})
```

- [ ] **Step 2: Run the focused test and verify missing API methods**

Run: `npm --prefix web run test -- --run src/api.test.ts`

Expected: FAIL because the capabilities/dev-login/provider methods and Vitest DOM setup are absent.

- [ ] **Step 3: Add the minimal harness and typed methods**

Add direct dev dependencies `@testing-library/react`, `@testing-library/jest-dom`, `@testing-library/user-event`, `jsdom`, `msw`, and `@playwright/test`. Configure Vitest with `environment: 'jsdom'` and `setupFiles: ['./src/test/setup.ts']`. Export typed `Capabilities`, `MessageProvider`, and `MessageProviderSettings` models. Keep authenticated requests adding `X-CSRF-Token` only for browser-cookie writes and `Authorization` only when a user explicitly supplied an automation token.

- [ ] **Step 4: Run API tests and TypeScript checks**

Run: `npm --prefix web run test -- --run src/api.test.ts` and `npm --prefix web run check`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/package.json web/package-lock.json web/vitest.config.ts web/src/test/setup.ts web/src/api.ts web/src/api.test.ts web/src/types.ts
git commit -m "test: add frontend API test harness"
```

### Task 2: Add login gate and message-provider settings UI

**Files:**
- Create: `web/src/pages/LoginPage.tsx`
- Create: `web/src/components/AuthGate.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/pages/SettingsPage.tsx`
- Create: `web/src/pages/LoginPage.test.tsx`
- Create: `web/src/pages/SettingsPage.test.tsx`
- Modify: `web/src/pages/ApprovalsPage.tsx`
- Modify: `web/src/pages/AccessPage.tsx`

**Interfaces:**
- `AuthGate` loads `api.capabilities()` and renders `LoginPage` when no active Session is available.
- `LoginPage` offers Feishu login only when `feishu_login` is true and local login only when `dev_login` is true.
- Settings displays four provider options, disables unavailable providers, and calls `api.selectMessageProvider` after confirmation.
- Approval requests no longer read `chatops_user_id` from `localStorage`; the backend derives the actor from the Session.
- Access management can list/add provider identities without exposing provider secrets.

- [ ] **Step 1: Write failing component tests**

```tsx
it('shows local development login but not Feishu login when capabilities allow only local auth', async () => {
  vi.spyOn(api, 'capabilities').mockResolvedValue({dev_login: true, feishu_login: false, message_providers: []})
  render(<MemoryRouter><LoginPage /></MemoryRouter>)
  expect(await screen.findByRole('button', {name: '本地开发登录'})).toBeVisible()
  expect(screen.queryByRole('link', {name: '飞书登录'})).toBeNull()
})

it('disables an unconfigured message provider', async () => {
  vi.spyOn(api, 'messageProvider').mockResolvedValue({active: 'web', providers: [{provider: 'web', configured: true, healthy: true, selectable: true}, {provider: 'dingtalk', configured: false, healthy: false, selectable: false, reason: 'not configured'}]})
  render(<SettingsPage />)
  expect(await screen.findByRole('option', {name: /钉钉/})).toBeDisabled()
})
```

- [ ] **Step 2: Run and observe missing component behavior**

Run: `npm --prefix web run test -- --run src/pages/LoginPage.test.tsx src/pages/SettingsPage.test.tsx`

Expected: FAIL because the login gate, capabilities loading, provider selector, and route test wrappers do not exist.

- [ ] **Step 3: Implement the UI with existing Ant Design conventions**

Keep the current dense operations layout. Add a full-width login surface with clear provider choices and error/loading states. Add a provider `Select` to `SettingsPage`, show configured/healthy status, require confirmation for switching, and invalidate the settings query after success. Add provider identity rows to `AccessPage` with provider and subject inputs, but never credential inputs. Use icons from `@ant-design/icons` for login, connection test, identity add, and switch actions. Keep text within controls at mobile widths.

- [ ] **Step 4: Run component, lint, and build checks**

Run: `npm --prefix web run test -- --run src/pages/LoginPage.test.tsx src/pages/SettingsPage.test.tsx`, `npm --prefix web run lint`, and `npm --prefix web run build`

Expected: PASS with no TypeScript or ESLint errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/LoginPage.tsx web/src/components/AuthGate.tsx web/src/App.tsx web/src/pages/SettingsPage.tsx web/src/pages/LoginPage.test.tsx web/src/pages/SettingsPage.test.tsx web/src/pages/ApprovalsPage.tsx
git commit -m "feat: add provider-aware console login and settings"
```

### Task 3: Add backend behavior and Kubernetes/Feishu regression tests

**Files:**
- Create: `internal/application/service_test.go`
- Create: `internal/auth/token_test.go`
- Create: `internal/adapter/kubernetes/manager_test.go`
- Create: `internal/transport/http/handler/api_test.go`
- Create: `internal/adapter/feishu/webhook_test.go`
- Create: `internal/testutil/postgres.go`

**Interfaces:**
- Tests use public behavior and a dedicated PostgreSQL database from `CHATOPS_TEST_DATABASE_URL`.
- Kubernetes tests use fake client reactors and do not require kubeconfig or a cluster.

- [ ] **Step 1: Write failing critical behavior tests**

Add table-driven tests for production self-approval, development direct queueing, illegal operation transitions, duplicate idempotency, invalid image prefix, Session CSRF rejection, API RBAC denial, Feishu signed command/callback handling, Kubernetes missing-container rejection, successful rollout status, and revision rollback cleanup.

- [ ] **Step 2: Run focused tests and record expected failures**

Run: `go test ./internal/application ./internal/auth ./internal/adapter/kubernetes ./internal/transport/http/handler ./internal/adapter/feishu -v`

Expected: FAIL for newly specified behavior that the current implementation does not prove. Correct setup errors before production edits.

- [ ] **Step 3: Implement only behavior fixes exposed by tests**

Map empty request IDs consistently, derive requester/approver solely from authenticated principals, preserve stable provider errors, ensure duplicate event IDs do not create a second operation, and make fake-client rollout checks deterministic. Do not broaden the public API beyond the approved contract.

- [ ] **Step 4: Run backend suites with and without PostgreSQL**

Run: `go test ./... -v` and `CHATOPS_TEST_DATABASE_URL=postgres://chatops:chatops@localhost:5432/chatops_test?sslmode=disable go test ./... -race -v`

Expected: the unit suite passes without a database; the CI-style run passes integration and race tests with PostgreSQL 17.

- [ ] **Step 5: Commit**

```bash
git add internal/application/service_test.go internal/auth/token_test.go internal/adapter/kubernetes/manager_test.go internal/transport/http/handler/api_test.go internal/adapter/feishu/webhook_test.go internal/testutil/postgres.go
git commit -m "test: cover deployment, auth, Kubernetes, and webhook behavior"
```

### Task 4: Make Docker Compose run the complete application

**Files:**
- Modify: `compose.yaml`
- Modify: `Dockerfile`
- Modify: `cmd/server/main.go`
- Create: `scripts/compose-smoke.sh`
- Modify: `.gitignore`

**Interfaces:**
- `chatops-deploy healthcheck` exits `0` only when `GET http://127.0.0.1:8080/readyz` returns 2xx.
- Compose app exposes host `8080`, depends on healthy PostgreSQL, and sets development/Web-only configuration.

- [ ] **Step 1: Write the failing Compose smoke script**

```bash
#!/usr/bin/env bash
set -euo pipefail
project=chatops-deploy-smoke
trap 'docker compose -p "$project" logs app || true; docker compose -p "$project" down --volumes' EXIT
docker compose -p "$project" up --build -d
for attempt in $(seq 1 60); do
  if curl --fail --silent http://localhost:8080/readyz >/dev/null; then break; fi
  sleep 2
done
curl --fail --silent http://localhost:8080/healthz >/dev/null
curl --fail --silent http://localhost:8080/ >/dev/null
curl --fail --silent http://localhost:8080/auth/capabilities | grep -q '"dev_login":true'
```

- [ ] **Step 2: Run the script and observe failure**

Run: `bash scripts/compose-smoke.sh`

Expected: FAIL because Compose has no `app` service, the binary has no probe command, and no capabilities endpoint exists in the image.

- [ ] **Step 3: Implement image and Compose changes**

Use `npm ci` with `web/package-lock.json` in the Dockerfile. Add the Go probe command before normal server startup. Add an app health check using `CMD ["/app/chatops-deploy", "healthcheck"]`, PostgreSQL `condition: service_healthy`, local-only defaults, port mapping, and a named database volume. Add the conversation export and smoke artifacts to `.gitignore` without removing existing entries.

- [ ] **Step 4: Run smoke and inspect status**

Run: `bash scripts/compose-smoke.sh`

Expected: both services become healthy and the four HTTP checks succeed before teardown.

- [ ] **Step 5: Commit**

```bash
git add compose.yaml Dockerfile cmd/server/main.go scripts/compose-smoke.sh .gitignore
git commit -m "feat: run the application with Docker Compose"
```

### Task 5: Add Playwright browser smoke and CI jobs

**Files:**
- Create: `web/playwright.config.ts`
- Create: `web/e2e/compose.spec.ts`
- Modify: `.github/workflows/ci.yml`
- Modify: `web/package.json`
- Modify: `web/package-lock.json`

**Interfaces:**
- Browser smoke uses `http://localhost:8080`, logs in locally, visits `/settings`, and checks all four provider options.
- CI backend starts PostgreSQL 17 and exports `CHATOPS_TEST_DATABASE_URL`.
- CI frontend runs unit tests in addition to type check, lint, and build.
- CI smoke runs only after backend, frontend, and image jobs pass.

- [ ] **Step 1: Write the failing browser test**

```ts
test('local operator can log in and see provider choices', async ({page}) => {
  await page.goto('http://localhost:8080/')
  await page.getByRole('button', {name: '本地开发登录'}).click()
  await expect(page).toHaveURL(/dashboard/)
  await page.goto('http://localhost:8080/settings')
  await expect(page.getByText('飞书')).toBeVisible()
  await expect(page.getByText('企业微信')).toBeVisible()
  await expect(page.getByText('钉钉')).toBeVisible()
  await expect(page.getByText('仅 Web 控制台')).toBeVisible()
  await page.screenshot({path: `test-results/settings-${test.info().project.name}.png`, fullPage: true})
})
```

- [ ] **Step 2: Run and observe missing browser setup**

Run: `npm --prefix web exec -- playwright test --config playwright.config.ts`

Expected: FAIL until the Playwright config, browser dependency, Compose server, and login UI exist.

- [ ] **Step 3: Implement Playwright and CI orchestration**

Configure Chromium projects for desktop (`1280x800`) and mobile (`390x844`), use `trace: 'retain-on-failure'`, capture screenshots on every run, and set a short web-first timeout. Add CI service PostgreSQL, backend race tests, frontend tests, image build, Compose startup, readiness polling, Playwright install, smoke test, artifact upload, and unconditional Compose teardown.

- [ ] **Step 4: Run CI-equivalent browser checks**

Run: `npm --prefix web exec -- playwright install chromium` and `npm --prefix web exec -- playwright test --config playwright.config.ts`

Expected: both viewport projects pass against Compose; screenshots show visible, non-overlapping main content.

- [ ] **Step 5: Commit**

```bash
git add web/playwright.config.ts web/e2e/compose.spec.ts web/package.json web/package-lock.json .github/workflows/ci.yml
git commit -m "ci: run frontend and Compose browser verification"
```

### Task 6: Document, verify, and push the feature branch

**Files:**
- Modify: `README.md`
- Modify: `.env.example`
- Modify: `config/config.example.yaml`
- Create: `docs/local-development.md`

- [ ] **Step 1: Write documentation checks**

```bash
rg -n 'docker compose up --build -d|/readyz|CHATOPS_DEV_AUTH_ENABLED|CHATOPS_MESSAGE_PROVIDER|Web-only|企业微信|钉钉|down --volumes' README.md docs/local-development.md .env.example
! git ls-files chatops-deploy-conversation-export.md
```

- [ ] **Step 2: Run checks and observe missing instructions**

Expected: FAIL until Compose, login, provider selection, reset, and secret instructions are documented.

- [ ] **Step 3: Add operator documentation**

Document Bash and PowerShell startup, status, logs, local login, stop, destructive volume reset, provider modes, required secret groups, browser-auth separation, and GitHub branch flow. Keep existing README content and review its CRLF diff separately.

- [ ] **Step 4: Run the complete verification matrix**

```bash
gofmt -w $(find . -name '*.go' -not -path './.tools/*')
go vet ./...
go test ./...
CHATOPS_TEST_DATABASE_URL=postgres://chatops:chatops@localhost:5432/chatops_test?sslmode=disable go test ./... -race
npm --prefix web ci
npm --prefix web run check
npm --prefix web run lint
npm --prefix web run test -- --run
npm --prefix web run build
bash scripts/compose-smoke.sh
```

Expected: all commands pass. Record exact environment-only skips rather than claiming an unrun check passed.

- [ ] **Step 5: Review and commit documentation**

Run `git status --short`, `git diff --cached --check`, and `git diff --cached --name-only`. Stage only task files; confirm `LICENSE`, unrelated line-ending-only changes, `.env`, generated artifacts, and conversation export are unstaged. Commit:

```bash
git add README.md docs/local-development.md .env.example config/config.example.yaml
git commit -m "docs: document local deployment and message providers"
```

- [ ] **Step 6: Push and verify the feature branch**

```bash
git push --set-upstream origin feature/local-compose-and-tests
git rev-parse HEAD
git ls-remote --heads origin feature/local-compose-and-tests
git status --short --branch
```

Expected: remote branch hash equals local `HEAD`, no force push occurs, `origin/main` is unchanged, and only pre-existing user-owned files remain unstaged. Report the compare/Pull Request URL without merging.
