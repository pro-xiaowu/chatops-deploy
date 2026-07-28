# Application Environment Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let administrators create and view Kubernetes publish environments from an application row in the React console.

**Architecture:** Keep the existing REST contract and add UI only in the Application Directory. A row action opens an application-scoped drawer, which loads that application's environments; a nested modal posts the existing `AppEnvironment` payload through `api.createEnvironment`. The Release Center continues to consume the existing `/environments` endpoint.

**Tech Stack:** React 19, TypeScript, Ant Design 5, TanStack Query 5, Vitest 3, Testing Library, Playwright.

## Global Constraints

- Preserve the existing `/api/v1/environments` API and its session/API-token plus CSRF authentication behavior.
- Do not render kubeconfig data or add cluster credential fields outside the existing Infrastructure flow.
- `production` must always require approval in the UI and remains enforced by the backend.
- Do not add environment edit/delete behavior, a top-level environment navigation item, or an application details route.
- Keep the existing dense console layout usable at desktop and mobile widths.
- Use Linux commands and do not stage `.env`, generated artifacts, or unrelated user-owned changes.

---

## File Structure

- `web/package.json`, `web/package-lock.json`: add the minimal DOM test dependencies.
- `web/vitest.config.ts`, `web/src/test/setup.ts`: configure jsdom and Testing Library matchers.
- `web/src/test/render.tsx`: wrap page tests with TanStack Query and React Router.
- `web/src/pages/ApplicationsPage.tsx`: own the row action, environment drawer, table, and creation modal.
- `web/src/pages/ApplicationsPage.test.tsx`: verify observable environment-management behavior.
- `web/e2e/compose.spec.ts`: extend authenticated desktop/mobile smoke coverage.
- `README.md`, `README.en.md`: document the actual configuration flow.

### Task 1: Add the Frontend Component Test Harness

**Files:**
- Modify: `web/package.json`
- Modify: `web/package-lock.json`
- Modify: `web/vitest.config.ts`
- Create: `web/src/test/setup.ts`
- Create: `web/src/test/render.tsx`

**Interfaces:**
- `renderConsole(ui: ReactElement): RenderResult` wraps the UI in `QueryClientProvider` and `MemoryRouter`.
- Vitest uses `environment: 'jsdom'` and loads `@testing-library/jest-dom/vitest`.

- [ ] **Step 1: Install only the test-environment dependencies**

Run:

```bash
npm --prefix web install --save-dev @testing-library/jest-dom@^6.6.3 @testing-library/react@^16.2.0 @testing-library/user-event@^14.6.1 jsdom@^26.0.0
```

Expected: `web/package.json` and `web/package-lock.json` contain exactly these development-only packages.

- [ ] **Step 2: Configure and verify the reusable test setup**

Set jsdom and `setupFiles: ['./src/test/setup.ts']` in `vitest.config.ts`. Import `@testing-library/jest-dom/vitest` and provide no-op `window.matchMedia` and `ResizeObserver` implementations in setup for Ant Design. Implement the real wrapper without mocking pages or Ant Design:

```tsx
export function renderConsole(ui: ReactElement) {
  const client = new QueryClient({
    defaultOptions: {queries: {retry: false}, mutations: {retry: false}},
  })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  )
}
```

- [ ] **Step 3: Prove the harness with the existing API suite**

Run:

```bash
npm --prefix web run test -- --run src/api.test.ts
```

Expected: PASS in jsdom, proving the environment did not break existing CSRF/client tests.

- [ ] **Step 4: Run TypeScript and lint checks**

```bash
npm --prefix web run check
npm --prefix web run lint
```

Expected: PASS with the new test utility included.

- [ ] **Step 5: Commit the test foundation**

```bash
git add web/package.json web/package-lock.json web/vitest.config.ts web/src/test/setup.ts web/src/test/render.tsx
git commit -m "test: add application page test harness"
```

### Task 2: Implement the Application-Scoped Environment Drawer

**Files:**
- Modify: `web/src/pages/ApplicationsPage.tsx`
- Create: `web/src/pages/ApplicationsPage.test.tsx`

**Interfaces:**
- `api.environments(applicationId)` returns the selected application's `Environment[]`.
- `api.clusters()` supplies cluster labels and form options.
- `api.createEnvironment(input)` receives all environment fields except server-assigned `id`.

- [ ] **Step 1: Write the failing application-row behavior test**

Mock only the API boundary, resolve one application and one cluster, click the absent row action, and assert the selected application scopes the drawer and request:

```tsx
it('opens an environment drawer scoped to the selected application', async () => {
  vi.mocked(api.applications).mockResolvedValue([
    {id: 'app-orders', name: 'orders-api', description: 'Orders', enabled: true},
  ])
  vi.mocked(api.clusters).mockResolvedValue([
    {id: 'cluster-prod', name: 'prod', api_server: 'https://k8s.example', credential_version: 1, enabled: true, last_check_status: 'healthy'},
  ])
  vi.mocked(api.environments).mockResolvedValue([])

  renderConsole(<ApplicationsPage />)
  await userEvent.click(await screen.findByRole('button', {name: '管理环境'}))

  expect(await screen.findByRole('dialog', {name: 'orders-api 环境'})).toBeVisible()
  expect(api.environments).toHaveBeenCalledWith('app-orders')
})
```

Run the focused suite and verify it fails because `管理环境` is absent, not because of test setup.

- [ ] **Step 2: Implement the minimal scoped drawer and verify green**

Add the operation column, selected-application state, drawer title, conditional `api.environments(selectedApplication.id)` query, and environment table. Run the focused suite and expect PASS.

- [ ] **Step 3: Add failing tests for payload and production approval**

Add a test that fills a `test` environment and asserts this literal payload:

```ts
{
  application_id: 'app-orders',
  name: 'test',
  cluster_id: 'cluster-prod',
  namespace: 'orders',
  deployment: 'orders-api',
  container: 'api',
  image_prefix: 'ghcr.io/acme/orders-api:',
  approval_required: false,
  rollout_timeout_seconds: 480,
}
```

Add a separate test that selects `production` and asserts the approval control is checked and disabled. These catch dropped workload fields and unsafe production configuration.

- [ ] **Step 4: Run the focused tests and verify they fail**

```bash
npm --prefix web run test -- --run src/pages/ApplicationsPage.test.tsx
```

Expected: FAIL because the drawer and form are absent.

- [ ] **Step 5: Implement the minimal creation form**

In `ApplicationsPage.tsx`:

1. Add `添加环境` with the existing plus icon. Open a modal containing environment/cluster `Select`, workload `Input` controls, timeout `InputNumber`, and approval `Switch`.
2. Default timeout to 600 and approval to false. When name is `production`, set approval true and disable the switch. Other names re-enable it.
3. Submit the exact form payload plus `application_id`. On success reset and close the modal, invalidate `['environments', selectedApplication.id]`, and show `环境已创建`.
4. On error show the server message with `message.error` and leave the form open.

Do not add routes, kubeconfig calls, edit/delete actions, or API changes.

- [ ] **Step 6: Run component tests and verify green**

Run the focused suite. Expected: all drawer, payload, approval, success-refresh, and error-presentation tests pass.

- [ ] **Step 7: Add the no-cluster test before its UI**

Make `api.clusters()` resolve to `[]`. Assert `请先在基础设施中注册 Kubernetes 集群` appears and `添加环境` is disabled. Verify RED, then render the prerequisite and disable the action. Verify GREEN.

- [ ] **Step 8: Commit drawer behavior**

```bash
git add web/src/pages/ApplicationsPage.tsx web/src/pages/ApplicationsPage.test.tsx
git commit -m "feat: manage application environments in console"
```

### Task 3: Verify Release Selection and Responsive Browser Flow

**Files:**
- Modify: `web/e2e/compose.spec.ts`
- Modify: `README.md`
- Modify: `README.en.md`

**Interfaces:**
- Development login establishes the real Session and CSRF flow.
- The Release Center already reads `GET /api/v1/environments`.

- [ ] **Step 1: Write a failing Playwright environment-management scenario**

After development login, create one application through the authenticated public API boundary, navigate to `/applications`, click `管理环境`, and assert the drawer and no-cluster prerequisite are visible:

```ts
await page.goto('/applications')
await expect(page.getByRole('button', {name: '管理环境'})).toBeVisible()
await page.getByRole('button', {name: '管理环境'}).click()
await expect(page.getByRole('dialog', {name: /环境/})).toBeVisible()
await expect(page.getByText('请先在基础设施中注册 Kubernetes 集群')).toBeVisible()
```

- [ ] **Step 2: Run the browser scenario and verify it fails for the missing UI**

```bash
docker compose up --build -d --wait
npm --prefix web run test:e2e -- --grep "environment management"
```

Expected: FAIL on the missing action/drawer before the latest image is built; do not weaken locators to hide missing behavior.

- [ ] **Step 3: Rebuild and verify desktop/mobile behavior**

Rebuild Compose with the implementation, run the Playwright scenario for both configured projects, and verify the drawer main content stays visible without horizontal overlap.

- [ ] **Step 4: Document the user flow**

Add `配置发布目标` to `README.md` and `Configure a Release Target` to `README.en.md`. Document this order: register cluster, add application, use `管理环境` to bind an existing Kubernetes workload, then select it in Release Center. State that ChatOps does not create the application or Kubernetes Deployment.

- [ ] **Step 5: Run complete frontend and Compose verification**

```bash
npm --prefix web run check
npm --prefix web run lint
npm --prefix web run test -- --run
npm --prefix web run build
docker compose up --build -d --wait
curl --fail --silent --show-error http://localhost:8080/readyz
npm --prefix web run test:e2e
```

Expected: every command passes and both Compose services remain healthy.

- [ ] **Step 6: Commit browser coverage and documentation**

```bash
git add web/e2e/compose.spec.ts README.md README.en.md
git commit -m "test: cover environment management browser flow"
```

### Task 4: Deliver and Verify the Feature Branch

**Files:**
- No additional source files.

**Interfaces:**
- Local branch `feature/local-compose-and-tests-impl` pushes to `origin/feature/local-compose-and-tests`.
- GitHub Actions validates frontend, backend, image, and Compose smoke jobs.

- [ ] **Step 1: Inspect the final ownership and diff**

```bash
git status --short --branch
git diff --check origin/feature/local-compose-and-tests...HEAD
git log --oneline origin/feature/local-compose-and-tests..HEAD
```

Expected: only planned files are committed, with no generated output, credentials, conversation export, or unrelated user changes.

- [ ] **Step 2: Push without changing main**

```bash
git push origin HEAD:feature/local-compose-and-tests
```

- [ ] **Step 3: Watch remote CI to completion**

```bash
gh run list --branch feature/local-compose-and-tests --limit 1
run_id=$(gh run list --branch feature/local-compose-and-tests --limit 1 --json databaseId --jq '.[0].databaseId')
gh run watch "$run_id" --exit-status
```

Expected: frontend, backend, image, and smoke jobs all succeed. If a job exposes a real defect, add a focused regression test, make the minimal fix, push again, and watch the replacement run.
