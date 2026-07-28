# chatops-deploy

[简体中文](README.md) | [English](README.en.md)

`chatops-deploy` is a ChatOps operations platform for Kubernetes Deployments. Operators can request deployments, rollbacks, approvals, and status checks from the web console, Feishu, WeCom, or DingTalk. PostgreSQL stores operation state, identity mappings, audit events, and reliable notifications. Production changes require approval from a different authorized user.

> Local deployment and development are supported on Linux only.

## Features

- `/deploy`, `/rollback`, `/approve`, `/status`, and `/help` ChatOps commands
- Selectable Feishu, WeCom, DingTalk, or web-only messaging
- Separate browser authentication and provider identity mappings, with multiple external identities per local user
- AES-256-GCM encrypted kubeconfig storage for multiple clusters
- Deployment image updates, rollout observation, and ReplicaSet revision rollback
- PostgreSQL task queue, approvals, auditing, transactional outbox, claim leases, and SSE events
- Application-level RBAC, Argon2id API tokens, and CSRF protection for browser writes
- Linux multi-stage Docker image, Docker Compose, Kubernetes manifests, and GitHub Actions

## Linux Quick Start

Install Docker Engine, Docker Compose v2, and `curl`. Ports `5432` and `8080` must be available.

```bash
docker compose up --build -d --wait
docker compose ps
curl --fail http://localhost:8080/readyz
```

Open http://localhost:8080 and select "Local development login". Compose enables a development-only session login and defaults to `web` messaging, so no third-party provider credentials are required.

```bash
docker compose logs -f app
docker compose down
```

To reset the local database, explicitly remove the data volume. This operation is irreversible:

```bash
docker compose down --volumes
```

`/healthz` reports process liveness. `/readyz` reports database connectivity and completed migrations. See the [Linux local development guide](docs/local-development.md) for complete configuration details.

## Message Providers

An administrator can select one configured provider after its health check succeeds. Changing the messaging provider does not change browser authentication. Existing operations always send results through the provider recorded when the operation was created.

| Mode | Inbound commands and approvals | Outbound notifications | Browser login |
| --- | --- | --- | --- |
| Web | Web console | Web console | Local development session or configured Feishu OAuth |
| Feishu | Signature and verification-token validation, including encrypted events | Text and interactive cards | OAuth supported |
| WeCom | Signed normalized JSON callback | Application messages | Not provided |
| DingTalk | Signed normalized JSON callback | Direct robot messages | Not provided |

Native encrypted WeCom XML or DingTalk event streams require an ingress gateway that decrypts and normalizes provider fields. Required environment variables and signing contracts are documented in the [Linux local development guide](docs/local-development.md).

## Testing

Backend coverage includes PostgreSQL migrations and storage, the operation state machine, provider signatures, the Kubernetes fake client, HTTP authentication, and the worker outbox. Frontend checks include TypeScript, linting, Vitest, and desktop/mobile Playwright tests in GitHub Actions.

```bash
go vet ./...
go test ./... -race
npm --prefix web ci
npm --prefix web run check
npm --prefix web run lint
npm --prefix web run test -- --run
npm --prefix web run build
bash scripts/compose-smoke.sh
```

Set `CHATOPS_TEST_DATABASE_URL` for PostgreSQL integration tests. CI starts an isolated PostgreSQL 17 service automatically.

## Kubernetes Deployment

Update the image and public URL in `deployments/kubernetes/deployment.yaml`, create `chatops-deploy-secrets` with the keys listed in `.env.example`, and apply the manifests:

```bash
kubectl apply -f deployments/kubernetes/namespace.yaml
kubectl apply -f deployments/kubernetes/serviceaccount-rbac.yaml
kubectl apply -f deployments/kubernetes/deployment.yaml
kubectl apply -f deployments/kubernetes/service.yaml
```

Deploy namespace-scoped RBAC in every managed namespace. The example `Role` is illustrative and should not be widened across namespaces.

## GitHub and GHCR

Pull requests and pushes to `main` or `feature/**` run Go, frontend, image, and Compose browser smoke tests. Pushing a strict semantic version tag such as `v0.1.0` publishes:

```text
ghcr.io/<GitHub owner>/chatops-deploy:v0.1.0
ghcr.io/<GitHub owner>/chatops-deploy:latest
```

The release workflow produces an SBOM and provenance, then signs the image with keyless Cosign through GitHub OIDC.

## Security Boundaries

- Applications and environments must be registered; users cannot submit arbitrary Kubernetes resources.
- Kubeconfigs, provider secrets, and tokens are never written to logs in plaintext.
- A production requester cannot approve their own operation.
- External callbacks without a valid signature or verification token are rejected.
- Workers only update the registered container in the selected Deployment, and images must match the registered repository prefix.
- Production deployments should restrict API, metrics, and database access with an ingress, NetworkPolicies, and a secret manager.
