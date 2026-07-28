# chatops-deploy 设计文档

日期：2026-07-28

## 1. 项目目标

`chatops-deploy` 是一个同时提供飞书群聊入口和 Web 运维控制台的 Kubernetes ChatOps 服务。用户通过群消息、交互卡片或控制台发起应用部署、回滚及状态查询；开发和测试环境可直接执行，生产环境必须由另一名授权审批人批准。系统使用本地 RBAC 控制用户可操作和可审批的应用范围，并对所有请求、审批、执行和通知保留完整审计记录。

首版接入飞书并提供完整 Web 运维控制台，只管理 Kubernetes Deployment，同时支持多集群。企业微信、StatefulSet、Helm、Kustomize、通讯录同步和 GitOps 不属于首版范围。

## 2. 技术栈

- Go 1.22 或更高版本
- Gin HTTP 框架
- Kubernetes client-go
- PostgreSQL 与 GORM
- 飞书开放平台事件订阅和交互卡片
- React、TypeScript、Vite 与 Ant Design
- React Router、TanStack Query 与 Server-Sent Events
- Viper 配置管理
- Zap 结构化日志
- Docker 多阶段构建
- GitHub Actions 持续集成

## 3. 总体架构

项目采用模块化单体和前后端同仓库结构：一个 Go module、一个 `web/` React 工程、一个 Go 可执行文件和一个 Docker 镜像。生产环境由 Gin 同域提供 API 和构建后的 SPA 静态资源。进程通过配置支持三种运行模式：

- `all`：同时运行 HTTP API、飞书 Webhook 和后台 Worker，作为首版默认模式。
- `api`：只运行 HTTP API 和飞书 Webhook。
- `worker`：只运行 Kubernetes 操作执行器及消息 outbox 发送器。

该模式允许首版保持简单，也允许后续不改代码便将入口服务与持有集群凭据的 Worker 分开部署。

建议的代码边界如下：

- `cmd/server`：进程启动、依赖装配和优雅停机。
- `internal/domain`：应用、环境、集群、用户、角色、操作单、审批和审计等领域模型，不依赖 Gin、GORM 或 client-go。
- `internal/application`：部署、回滚、审批、状态查询和管理用例，负责权限校验与事务边界。
- `internal/adapter/feishu`：事件验签与解密、消息解析、交互卡片、回调处理和结果通知。
- `internal/adapter/kubernetes`：多集群客户端管理、Deployment 更新、rollout 观察和 revision 回滚。
- `internal/adapter/postgres`：GORM 仓储、事务、任务领取、outbox 和迁移。
- `internal/transport/http`：Gin 路由、Webhook、管理 API 和 HTTP 中间件。
- `internal/transport/sse`：经权限过滤的操作和 rollout 实时事件流。
- `internal/worker`：任务领取、目标租约、重试、超时、执行结果和通知调度。
- `web/`：React 运维控制台、路由、页面、组件、生成的 API Client 和前端测试。

领域层通过小型接口表达仓储、Kubernetes 执行器和消息发送器等依赖。适配器实现这些接口，确保核心业务规则可在不启动外部组件的情况下测试。

## 4. 数据模型

核心数据表如下：

### 4.1 身份与权限

- `users`：本地用户及飞书 `open_id`，包含启用状态。
- `roles`：管理员、操作者、审批者等角色。
- `user_roles`：用户与角色关联。
- `app_permissions`：用户或角色对特定应用的操作权和审批权。
- `api_tokens`：管理 API Token 的名称、指纹、安全哈希、权限、过期时间和吊销时间。
- `web_sessions`：Web 会话哈希、用户、过期时间、最后活动时间和撤销时间。
- `oauth_states`：一次性飞书 OAuth state、过期时间和使用状态。

群聊请求与 Web 登录均以飞书 `open_id` 映射本地用户。未知、停用或无权限用户不能发起操作、审批或进入控制台。生产变更的请求人不能审批自己的操作。

### 4.2 集群与应用注册

- `clusters`：集群名称、状态、API Server 元数据、加密 kubeconfig、凭据版本和最后一次连通性检查结果。
- `applications`：应用名称、描述和启用状态。
- `app_environments`：应用在一个环境中的集群、命名空间、Deployment、容器名、允许的镜像仓库前缀、审批策略和 rollout 超时时间。

每个应用环境只能绑定一个 Deployment 和一个目标容器。集群 kubeconfig 使用 AES-256-GCM 加密，主密钥只通过环境变量或 Kubernetes Secret 注入，不写入数据库、日志或配置文件。

### 4.3 操作与审计

- `operations`：部署或回滚操作单，记录请求人、目标、期望镜像或 revision、状态、幂等键、执行次数、时间戳和脱敏错误摘要。
- `approvals`：审批人、决定、意见和审批时间。
- `audit_events`：追加写入的审计轨迹，记录操作者、动作、目标、请求摘要、状态变化和结果。
- `operation_events`：带单调递增 ID的操作、审批和 rollout 进度事件，供 Web 实时推送与断线续传。
- `inbox_events`：已接收的飞书事件或卡片回调 ID，用于去重。
- `outbox_messages`：待发送的飞书消息、尝试次数、下次重试时间和发送结果。
- `target_leases`：应用环境级的执行租约，防止同一目标发生并发变更。

数据库变更使用显式、可回退审查的版本化 SQL 迁移。生产环境不使用 GORM AutoMigrate 修改结构。

## 5. 操作状态机

需要审批的操作按以下状态流转：

```text
pending_approval -> approved -> queued -> running -> succeeded
                                      \-> failed
pending_approval -> rejected
pending_approval -> expired
```

开发和测试环境不创建待审批环节，而是从创建直接进入 `queued`。审批超时进入 `expired`；执行超时进入 `failed`。系统不在 rollout 失败时自动回滚，以免掩盖实际故障；用户可以在确认原因后发起显式回滚。

每次状态迁移必须在 PostgreSQL 事务中完成，事务同时校验前置状态并写入审计事件。非法或重复迁移返回原操作状态，不产生第二个执行任务。Worker 使用 `FOR UPDATE SKIP LOCKED` 领取任务，并以操作 ID作为幂等键。

## 6. 飞书交互

飞书适配器支持 URL 校验、消息事件、交互卡片回调、事件加密和签名验证。服务校验时间戳、签名、Verification Token 和 Encrypt Key。Webhook 在完成验签、事件去重及必要落库后立即响应；Kubernetes 操作不得阻塞回调请求。

支持以下命令：

- `/deploy`：返回部署卡片，用户选择应用、环境并输入镜像标签。
- `/rollback`：返回回滚卡片，用户选择应用、环境及可用 revision。
- `/status`：返回应用环境选择卡片，并查询 Deployment、Pod、镜像及 rollout 状态。
- `/help`：返回当前用户有权限使用的命令入口。

卡片中的应用和环境选项只展示当前用户有权限操作的预注册资源。服务端仍会在提交和回调时重新校验身份、权限、目标状态、镜像规则和禁止自批规则，不能信任客户端卡片状态。

生产操作创建后发送审批卡片。批准后进入执行队列；拒绝、过期、执行成功或失败后更新原卡片，并在原群发送简洁结果摘要。状态查询由独立的短时只读任务异步完成，不进入变更状态机，但会写入查询审计。事件 ID、回调 ID和业务幂等键均持久化去重。所有出站消息先写入 PostgreSQL outbox，失败后按指数退避重试；超过最大次数时保留失败状态和可观测指标，供管理员重放。

服务使用飞书 App ID和 App Secret 获取并缓存 tenant access token，缓存过期前主动刷新，鉴权失败时只允许强制刷新并重试一次，避免无限重试掩盖配置错误。

## 7. Web 运维控制台

控制台使用 React、TypeScript、Vite、Ant Design、React Router 和 TanStack Query。生产构建由 Gin 在同一域名下提供，前端路由统一回退到 `index.html`，API 固定使用 `/api/v1`。控制台采用面向重复运维操作的紧凑布局：固定侧栏、顶部环境与用户区域、可扫描的数据表格、明确的状态色和危险操作确认，不使用营销式首页或装饰性大卡片。

### 7.1 页面范围

- 总览：集群健康、应用环境状态、待审批数量、执行中任务和最近变更。
- 应用：应用列表、环境绑定、实时 Deployment 与 Pod 状态、当前版本和发布历史。
- 发布中心：部署与回滚向导、操作列表、操作详情和实时 rollout 进度。
- 审批中心：待我审批、我发起的请求、批准、拒绝和审批意见。
- 基础设施：集群注册、连通性测试、凭据轮换、启用和停用。
- 访问控制：用户、角色、应用权限和 API Token。
- 审计日志：按用户、应用、集群、操作类型、状态和时间筛选。
- 系统设置：飞书连接状态、任务参数和非敏感运行配置。

网页和飞书共用相同的应用层用例、RBAC、审批状态机和审计逻辑。生产操作无论从哪个入口发起，都必须由另一名授权用户审批；审批可在网页或飞书完成，两个入口展示相同的持久化状态。

### 7.2 实时更新

控制台通过 Server-Sent Events 接收操作状态、审批和 rollout 更新。事件先写入 `operation_events`，默认保留 24 小时，保留时间可配置。客户端断线重连时携带最后事件 ID，服务端从持久化游标续传；超出保留窗口时返回明确事件，要求客户端刷新相关查询。SSE 连接按当前用户的应用权限过滤，普通列表和详情仍由 TanStack Query 拉取、缓存和失效更新。

### 7.3 飞书 OAuth 与会话安全

用户点击飞书登录后，后端创建短时、一次性的 OAuth `state` 并跳转飞书授权页。回调成功后取得飞书身份，以 `open_id` 匹配已有本地用户；首次登录不会自动创建授权角色。

Web Session ID由密码学安全随机源生成，PostgreSQL 只保存哈希。浏览器 Cookie 设置 `HttpOnly`、`Secure` 和 `SameSite=Lax`。所有写请求额外校验 CSRF Token、`Origin` 和 `Content-Type`；登录、审批、Token 签发和集群凭据变更有独立速率限制。退出登录、用户停用或管理员撤销会话后立即失效。

浏览器使用 OAuth 会话访问 API，自动化调用使用 Bearer API Token，两种认证不能相互降级。前端不得读取或持久化飞书 App Secret、集群 kubeconfig、API Token 明文或 kubeconfig 加密主密钥。敏感值使用只写接口，响应只返回指纹、版本和更新时间。SSE 使用同源会话认证。

## 8. Kubernetes 执行

### 8.1 多集群客户端

每个启用集群对应独立的 client-go ClientSet，并按集群 ID与凭据版本缓存。首次使用时解密 kubeconfig、构建客户端并进行连通性检查；轮换凭据或停用集群会使缓存失效。服务不会将 kubeconfig 内容写入日志或错误响应。

集群侧身份遵循最小权限原则，只授予已注册命名空间内读取 Deployment、ReplicaSet 和 Pod，以及读取和更新 Deployment 所需的权限。

### 8.2 部署

执行器读取最新 Deployment，依次验证：

1. Deployment 和目标容器存在。
2. 目标镜像符合该应用环境配置的仓库前缀。
3. 当前操作仍持有应用环境租约。

执行器使用 client-go 冲突重试更新目标容器镜像，记录变更前后的镜像、generation 和 resourceVersion，然后等待 rollout。成功条件包括 observedGeneration 已追上目标 generation、updatedReplicas 达到期望副本数、availableReplicas 达到期望副本数且 Progressing condition 未失败。

### 8.3 回滚

执行器读取 Deployment 关联的 ReplicaSet，根据 `deployment.kubernetes.io/revision` 注解生成可用历史 revision 列表。回滚时读取所选 ReplicaSet 的 PodTemplateSpec，移除 `pod-template-hash` 等 ReplicaSet 控制器生成的标签和注解，再通过 client-go 将清理后的模板应用到 Deployment；系统不调用 shell 或 `kubectl`。更新后等待一个新的 rollout 完成并记录实际生成的新 revision。

### 8.4 并发与错误分类

同一应用环境同一时间只能有一个 `running` 变更。Worker 在数据库中获取带过期时间、心跳和单调递增 fencing token 的目标租约。每次 Kubernetes 写操作和冲突重试前都必须重新核对租约所有者及 fencing token；失去租约的 Worker 不再提交新的 Kubernetes 写操作。写入 Deployment 时同时记录操作 ID注解，便于识别重试及追溯变更来源。

网络超时、API 限流和临时服务不可用属于可重试错误，使用有限次数的指数退避。权限不足、资源不存在、容器不存在、镜像不合规、无效 revision 和业务状态冲突直接失败。群消息只展示稳定的错误码和简洁说明；完整错误链进入 Zap 日志和受权限保护的审计详情，并对 Token、kubeconfig 和密钥进行脱敏。

## 9. 管理 API

Gin 暴露版本化管理接口 `/api/v1`，包括：

- 集群注册、列表、连通性测试、凭据轮换、启用和停用。
- 应用及环境绑定的增删改查。
- 用户、角色和应用权限管理。
- 管理 API Token 的签发、列表、吊销和轮换。
- 操作单、审批记录、outbox 失败记录和审计事件查询。

管理 Token 使用密码学安全随机源生成，只在创建时返回一次。数据库保存可检索的 SHA-256 指纹和 Argon2id 验证值，不保存 Token 明文。请求使用 Bearer Token 认证，并按 Token 关联的管理员权限授权。首次启动时，仅当数据库中不存在管理 Token，服务才接受 `CHATOPS_BOOTSTRAP_ADMIN_TOKEN` 创建初始管理员凭据；引导值不会被记录，已有 Token 时该环境变量被忽略并产生安全告警。

所有写接口校验 `Content-Type`、请求体大小、字段格式和资源状态，并支持 `Idempotency-Key`。中间件生成或接受合法的 `X-Request-ID`。错误响应采用稳定的业务错误码、用户可读消息和 request ID，不暴露内部堆栈。

所有控制台接口纳入仓库内的 OpenAPI 3 规范。前端请求类型和 API Client 由该规范生成，CI 校验规范、生成代码和后端响应模型没有漂移。

## 10. 配置、日志与可观测性

Viper 加载配置文件中的非敏感默认值，并允许以环境变量覆盖。数据库密码、飞书 App Secret、Verification Token、Encrypt Key、kubeconfig 加密主密钥和初始管理 Token 等秘密只能通过环境变量或挂载 Secret 提供。

Zap 以 JSON 输出结构化日志。请求与任务上下文统一携带适用的 `request_id`、`event_id`、`operation_id`、`user_id`、`app_id`、`environment_id` 和 `cluster_id`。

服务暴露：

- `/healthz`：仅表示进程存活。
- `/readyz`：检查数据库连接、迁移版本和当前运行模式所需组件。
- `/metrics`：输出 HTTP 请求、Webhook 去重、审批结果、任务积压、outbox、Kubernetes 操作耗时和结果等 Prometheus 指标。

进程收到终止信号后先停止接收新请求和新任务，再等待正在执行的任务到达安全点，释放或停止续租目标租约，最后关闭数据库和 HTTP 连接。

## 11. 测试策略

- 领域与应用层使用表驱动测试，覆盖 RBAC、禁止自批、状态迁移、幂等、审批过期、执行超时和错误分类。
- PostgreSQL 集成测试覆盖 GORM 映射、事务、唯一约束、`FOR UPDATE SKIP LOCKED`、目标租约、inbox 和 outbox。
- Kubernetes 单元测试使用 client-go fake；关键集成测试使用 envtest 或 Kind 验证 Deployment 更新、冲突重试、rollout 判断、并发保护和 revision 回滚。
- 飞书测试使用 `httptest` 覆盖 URL 校验、签名、加密事件、重复投递、命令解析、卡片回调和发送重试。
- Gin API 测试覆盖 Token 认证、授权、请求校验、幂等写入、错误格式和敏感信息保护。
- OAuth 与会话测试覆盖 state 重放、Cookie 属性、CSRF、会话撤销和停用用户。
- React 使用组件与页面测试覆盖权限路由、表单校验、部署与回滚向导、审批操作、SSE 重连和错误状态；关键浏览器流程使用 Playwright 验证。

## 12. 构建、部署与 GitHub CI/CD

源码、前端、SQL 迁移、Dockerfile、部署示例和文档存放在同一个 GitHub 仓库。Dockerfile 使用 Node 阶段构建 React 静态资源，使用 Go 1.22+ 阶段执行可复现编译，关闭 CGO 并生成静态二进制。运行阶段使用 distroless nonroot 镜像，包含 CA 证书、SPA 静态资源和嵌入二进制的 SQL 迁移，不包含 Node、Go 工具链或 shell。

GitHub Actions 包含两条流水线。

`ci.yml` 在 Pull Request 和主分支提交时执行：

1. Go 格式检查和模块一致性检查。
2. `go vet`、单元测试和 race detector。
3. PostgreSQL 与 Kubernetes 关键集成测试。
4. `govulncheck` 和依赖安全检查。
5. 前端类型检查、ESLint、单元测试和生产构建。
6. OpenAPI 规范与生成代码一致性检查。
7. 完整 Docker 镜像构建，不推送镜像。

`release.yml` 在推送版本 tag 时触发，并在发布前严格校验 tag 符合 `vMAJOR.MINOR.PATCH`：

1. 构建 `linux/amd64` 和 `linux/arm64` 镜像。
2. 使用 GitHub Actions 的 `github.repository_owner` 上下文发布到对应所有者名下的 `chatops-deploy` GHCR 包。
3. 生成语义化版本、Git SHA 和稳定版 `latest` 标签。
4. 生成 OCI 元数据、SBOM 和构建 provenance。
5. 使用 GitHub OIDC 完成 keyless 镜像签名。

发布工作流只授予 `contents: read`、`packages: write` 和 `id-token: write`，不保存长期 GHCR 密码。只有受保护的版本 tag 才允许发布镜像。README 给出创建 GitHub 仓库、推送源码、启用 Actions、拉取 GHCR 镜像和部署的完整步骤；实际首次推送需要仓库所有者提供目标仓库地址和 GitHub 授权。

## 13. 首版验收标准

首版在真实飞书群和至少两个 Kubernetes 集群上满足以下条件：

1. 授权用户能够通过 `/deploy` 卡片选择预注册应用、环境和镜像标签。
2. 开发和测试环境无需审批即可执行；生产环境必须由另一名具有对应应用审批权的用户批准。
3. 重复事件、重复卡片点击和 Worker 重试不会产生重复部署或回滚。
4. 部署能够更新指定容器镜像、正确判断 rollout 并回传结果。
5. 回滚能够列出并应用有效 revision，完成新的 rollout 并回传结果。
6. `/status` 能返回 Deployment、Pod、镜像、generation 和 rollout 摘要。
7. 无权限用户、越权应用、非法镜像、无效 revision 和同目标并发变更会被拒绝。
8. 飞书或 Kubernetes 临时故障按规则重试，永久错误可从操作单和审计记录定位。
9. 从请求、审批、执行到通知的完整链路可通过操作 ID追溯。
10. 容器以非 root 用户运行，秘密不出现在数据库明文字段、日志、HTTP 错误或飞书消息中。
11. 用户可使用飞书 OAuth 登录控制台，并且页面、数据、SSE 事件和操作均受本地 RBAC 限制。
12. 控制台覆盖总览、应用、发布、审批、集群、权限和审计的完整日常流程，生产审批与飞书入口保持一致。
13. 推送版本 tag 后，GitHub Actions 能发布带版本、SHA、SBOM、provenance 和签名的 amd64/arm64 GHCR 镜像。

## 14. 明确不做

首版不实现企业微信 Bot、企业通讯录同步、StatefulSet、DaemonSet、Helm、Kustomize、任意 manifest 执行、自动失败回滚和 GitOps 仓库写入。Web 控制台不提供源码上传或 GitHub 仓库管理功能；源码由标准 Git 工作流推送，镜像由 GitHub Actions 发布。这些未包含能力可在首版边界稳定后通过现有应用层接口和运行模式逐步扩展。
