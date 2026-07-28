# Local Compose Deployment and Test Completion Roadmap

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (recommended for this bounded sequence) or superpowers:subagent-driven-development to implement the phase plans task-by-task. Every implementation step follows TDD: write the failing behavior test, run it, implement the minimum behavior, run the focused test, then run the relevant regression suite.

**Goal:** Make `chatops-deploy` runnable with Docker Compose, support administrator-selected Feishu/WeCom/DingTalk/Web-only messaging, add development-only local authentication, expand automated tests, and push the reviewed feature branch to GitHub.

**Architecture:** Keep the Go API, worker, PostgreSQL store, Kubernetes manager, and React console in one repository and one production-style image. Put provider-specific callback and notification behavior behind a normalized messaging interface; persist the active provider and external identities in PostgreSQL while keeping provider secrets in environment variables or mounted secrets.

**Tech Stack:** Go 1.25 module, Gin, GORM/PostgreSQL 17, client-go fake clients, React 19, TypeScript, Vite, Ant Design, Vitest, Testing Library, Playwright, Docker Compose, GitHub Actions.

## Global Constraints

- The supported runtime is Linux; Compose, shell scripts, and CI verification target Linux only.
- `CHATOPS_RUNTIME_ENV` accepts only `production` or `development`, defaulting to `production`.
- `CHATOPS_DEV_AUTH_ENABLED=true` is valid only with `CHATOPS_RUNTIME_ENV=development`.
- `CHATOPS_MESSAGE_PROVIDER` accepts only `web`, `feishu`, `wecom`, or `dingtalk`, defaulting to `web` for local Compose.
- Only one message provider is active at a time; provider credentials remain outside PostgreSQL.
- Browser authentication remains separate from message delivery; no WeCom/DingTalk browser OAuth is introduced.
- `chatops-deploy-conversation-export.md`, `.env`, kubeconfigs, generated secrets, coverage output, and Playwright artifacts are never committed.
- Every production code change has a behavior test that was observed failing before the implementation was written.
- Existing `LICENSE` and CRLF-only `README.md` changes are preserved and are not reset or mixed into unrelated commits.
- Work is committed on `feature/local-compose-and-tests` and pushed without force-updating `origin/main`.

## Phase Order

1. [Phase 1: Runtime and identity foundation](2026-07-28-phase-1-runtime-identity-plan.md) - configuration, migrations, normalized identities, provider setting storage, and idempotent local administrator provisioning.
2. [Phase 2: Message platform and API integration](2026-07-28-phase-2-message-platform-plan.md) - provider contract, Web-only/Feishu/WeCom/DingTalk adapters, generic webhook routing, development login, and admin provider settings API.
3. [Phase 3: Console, Compose, tests, and delivery](2026-07-28-phase-3-console-compose-ci-plan.md) - login/settings UI, backend/frontend/integration tests, Docker Compose, CI, documentation, branch push, and remote verification.

Each phase ends with a focused test suite and a separate commit. Phase 2 must not start until Phase 1 migrations and store interfaces are stable. Phase 3 must not start until the generic provider API and development Session flow are callable through `httptest`.

## Cross-Phase Verification

Run these from the repository root after the phase-specific checks:

```bash
gofmt -d $(find . -name '*.go' -not -path './.tools/*')
go vet ./...
go test ./...
npm --prefix web ci
npm --prefix web run check
npm --prefix web run lint
npm --prefix web run test -- --run
npm --prefix web run build
```

When Docker is available, also run:

```bash
docker compose up --build -d
curl --fail http://localhost:8080/healthz
curl --fail http://localhost:8080/readyz
docker compose down
```
