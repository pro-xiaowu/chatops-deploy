# Application Environment Management Design

Date: 2026-07-28

## Context

The API already supports creating and listing application environments. An environment maps a registered application to one Kubernetes workload: cluster, namespace, Deployment, container, allowed image prefix, approval policy, and rollout timeout. The console exposes application and cluster registration, but it does not expose that environment mapping. As a result, the Release Center has no target environments to select.

## Goal

Expose environment management in the existing Application Directory so an administrator can configure publish targets without using the API directly.

## Non-Goals

- Add a new top-level navigation entry or a separate application detail route.
- Change the existing environment API, Kubernetes execution flow, RBAC model, or production approval rule.
- Test connectivity to a Kubernetes cluster during environment creation.
- Add environment editing or deletion in this increment.

## Chosen Interaction

Add a `Manage environments` action to each Application Directory row. Activating it opens a right-side drawer scoped to that application. The drawer has two views:

1. An environment table listing the application's existing publish targets. It shows environment name, registered cluster, namespace, Deployment, container, image prefix, approval requirement, and rollout timeout.
2. An `Add environment` action that opens a form modal while preserving the current application context.

This keeps the configuration next to the application it governs and avoids a new navigation layer for a relationship users configure as part of application setup.

## Create Environment Form

The form requires an existing registered cluster and accepts:

- Environment: `development`, `test`, or `production`.
- Cluster: selected from registered clusters.
- Namespace, Deployment, and container name.
- Image prefix, for example `ghcr.io/acme/orders-api:`.
- Rollout timeout in seconds, defaulting to 600.
- Approval requirement.

Selecting `production` forces approval on and disables the toggle. This mirrors the server-side invariant, which always requires approval for a production environment. Development and test environments keep the administrator-selected value.

If no cluster has been registered, the drawer explains that an administrator must first use Infrastructure to register a cluster. It disables environment creation because a target cannot be valid without a cluster.

## Data Flow and Error Handling

Opening the drawer loads environments using the selected application ID and loads registered clusters for form choices. Submitting sends the existing `POST /api/v1/environments` request through `api.createEnvironment`. A successful response closes and resets the form, refreshes the selected application's environment list, and shows the existing success feedback pattern.

Requests retain the existing session or API-token authentication and CSRF handling. Server validation errors remain visible through the existing mutation error presentation. No kubeconfig or sensitive cluster credential is rendered in the drawer.

The Release Center already retrieves `/environments`; after an environment is created, it appears the next time that data is fetched without any API change.

## Test Design

Add component-level frontend tests using the existing React stack plus the smallest required browser-like test setup. Tests cover these observable behaviors:

- An application row opens its scoped environment drawer and requests only that application's environments.
- The new environment form sends the selected cluster and all workload fields to the existing API client.
- Selecting production makes approval required and prevents disabling it.
- A missing cluster disables the creation action and presents the prerequisite.
- A successful creation refreshes the environment list.

API tests remain responsible for the CSRF request contract; no duplicated implementation-level API assertions are added. An end-to-end smoke test will confirm the drawer is reachable after development login and that the responsive layout remains usable on desktop and mobile.

## Acceptance Criteria

- An administrator can create an environment from the Application Directory without using curl or raw API calls.
- The created environment is visible in the application's drawer and selectable in the Release Center.
- Production environments always require approval in both UI and backend.
- The UI makes the missing-cluster prerequisite actionable and does not submit invalid data.
- Existing frontend checks, unit tests, browser smoke tests, Docker Compose startup, and CI remain green.
