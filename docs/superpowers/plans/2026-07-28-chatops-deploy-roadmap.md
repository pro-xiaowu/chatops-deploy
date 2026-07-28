# chatops-deploy Implementation Roadmap

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement each phase plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a production-oriented Kubernetes ChatOps platform with Feishu workflows, a React operations console, PostgreSQL-backed approvals and audit, multi-cluster client-go execution, and signed GHCR releases.

**Architecture:** Build one modular Go service and one React SPA in a monorepo. Deliver the system in six independently testable phases; every phase ends with a runnable artifact and a clean verification command before the next phase begins.

**Tech Stack:** Go 1.22+, Gin, GORM, PostgreSQL, client-go, Viper, Zap, React, TypeScript, Vite, Ant Design, TanStack Query, OpenAPI, Docker, GitHub Actions

---

## Locked Repository Structure

```text
chatops-deploy/
├── api/openapi.yaml                  # HTTP contract and generated-client source
├── cmd/server/main.go                # Composition root and lifecycle
├── config/config.example.yaml        # Non-secret defaults
├── deployments/kubernetes/           # Namespace, RBAC, ConfigMap, Secret example, Deployment, Service
├── docs/superpowers/                  # Approved design and implementation plans
├── internal/
│   ├── adapter/feishu/                # Feishu event, card, OAuth and API client
│   ├── adapter/kubernetes/            # Multi-cluster client-go adapter
│   ├── application/                   # Use cases and transaction orchestration
│   ├── config/                        # Viper loading and validation
│   ├── domain/                        # Framework-independent models and rules
│   ├── logging/                       # Zap construction and context fields
│   ├── store/postgres/                # GORM stores, migrations, inbox/outbox and leases
│   ├── transport/http/                # Gin handlers and middleware
│   ├── transport/sse/                 # Permission-filtered durable event stream
│   └── worker/                        # Operation and outbox workers
├── migrations/                        # Versioned PostgreSQL SQL migrations
├── web/                               # React console and generated API client
├── .github/workflows/                 # CI and signed GHCR release workflows
├── Dockerfile
├── Makefile
├── compose.yaml
├── go.mod
└── README.md
```

## Phase Sequence

### Phase 1: Foundation, Domain, PostgreSQL, and Catalog API

Plan: `docs/superpowers/plans/2026-07-28-phase-1-foundation-catalog.md`

Deliver a runnable Gin service with validated Viper configuration, Zap logging, health endpoints, explicit migrations, AES-256-GCM kubeconfig encryption, domain state rules, API Token authentication, and CRUD APIs for clusters, applications, environments, users, roles, and permissions. PostgreSQL integration tests prove transactions and constraints.

Exit command:

```powershell
go test ./internal/domain/... ./internal/config/... ./internal/store/postgres/... ./internal/application/... ./internal/transport/http/... -race
```

### Phase 2: Kubernetes Operation Engine

Plan: `docs/superpowers/plans/2026-07-28-phase-2-kubernetes-worker.md`

Consume encrypted kubeconfig through credential-versioned client caching, then add Deployment status, image deployment, revision discovery, rollback template sanitization, rollout observation, PostgreSQL job claiming, fencing leases, operation events, retries, and audit records. A Kind suite proves deploy, rollback, concurrency rejection, and timeout behavior.

Exit command:

```powershell
go test ./internal/adapter/kubernetes/... ./internal/worker/... -race
```

### Phase 3: Feishu Bot, Approval, OAuth, and Outbox

Plan: `docs/superpowers/plans/2026-07-28-phase-3-feishu-auth.md`

Add verified and encrypted Feishu callbacks, command cards, approval callbacks, tenant-token caching, inbox deduplication, durable outbox delivery, OAuth login, hashed Web sessions, CSRF protection, and cross-channel approval consistency. HTTP tests use deterministic Feishu fixtures and a fake Feishu server.

Exit command:

```powershell
go test ./internal/adapter/feishu/... ./internal/application/... ./internal/transport/http/... ./internal/worker/... -race
```

### Phase 4: React Operations Console

Plan: `docs/superpowers/plans/2026-07-28-phase-4-web-console.md`

Add the authenticated operations shell, dashboard, applications, release workflow, approvals, clusters, access control, audit log, system settings, generated OpenAPI client, and durable SSE updates. Component tests and Playwright cover the main daily workflows and responsive layouts.

Exit command:

```powershell
npm --prefix web run check
npm --prefix web run test
npm --prefix web run build
npm --prefix web run test:e2e
```

### Phase 5: Observability and Production Packaging

Plan: `docs/superpowers/plans/2026-07-28-phase-5-production.md`

Add Prometheus metrics, readiness dependency checks, graceful shutdown, structured redaction tests, Node/Go/distroless multi-stage build, local Compose, Kubernetes manifests, least-privilege RBAC, secret examples, and smoke tests against the built image.

Exit command:

```powershell
docker compose up -d postgres
docker build -t chatops-deploy:verify .
docker run --rm chatops-deploy:verify --version
```

### Phase 6: GitHub CI and Signed GHCR Release

Plan: `docs/superpowers/plans/2026-07-28-phase-6-github-release.md`

Add PR CI, integration services, generated-code drift checks, multi-architecture GHCR publishing, SBOM, provenance, Cosign keyless signing, semantic tag validation, Dependabot, release documentation, and a final clean-clone verification.

Exit command:

```powershell
go test ./... -race
npm --prefix web run check
npm --prefix web run test
npm --prefix web run build
docker build -t chatops-deploy:verify .
```

## Delivery Rules

1. Implement phases in order because later phases consume contracts established earlier.
2. Do not begin a phase until the previous exit command passes.
3. Use TDD for every behavior change: failing focused test, minimal implementation, focused pass, broader pass, commit.
4. Keep migrations append-only after a phase is merged; never edit a migration already exercised outside a local disposable database.
5. Commit generated OpenAPI client changes with the contract that generated them.
6. Never commit credentials, kubeconfig data, OAuth secrets, API Tokens, private signing keys, `.env`, or local database volumes.
7. The first external GitHub push occurs only after the user supplies the repository URL and authorizes the push.
