# chatops-deploy

在飞书群或 Web 运维控制台中创建 Kubernetes Deployment 的部署、回滚、审批与状态查询操作。服务使用 PostgreSQL 持久化操作状态和审计记录，使用 `client-go` 操作预注册应用，生产环境变更必须经过另一名授权用户审批。

## 功能

- 飞书 `/deploy`、`/rollback`、`/status` 和 `/help` 命令入口
- 飞书 OAuth 登录和同域 Web 控制台
- 多集群 kubeconfig AES-256-GCM 加密存储
- Deployment 镜像部署、rollout 观察、ReplicaSet revision 回滚
- PostgreSQL 任务队列、审批、审计、outbox 和 SSE 事件
- 管理 API Token 使用 Argon2id 哈希，浏览器写请求使用 CSRF Token
- Docker 多阶段构建、Kubernetes 清单、GitHub CI 与 GHCR 多架构发布

## 本地运行

需要 Go 1.22+、Node 22+、Docker 和 PostgreSQL 17+。

```powershell
docker compose up -d postgres
Copy-Item .env.example .env
```

生成 32 字节主密钥并写入 `CHATOPS_SECURITY_KUBECONFIG_MASTER_KEY`：

```powershell
[Convert]::ToBase64String((1..32 | ForEach-Object { Get-Random -Maximum 256 }))
```

设置数据库 URL、飞书凭据和引导管理 Token 后运行：

```powershell
go run ./cmd/server
npm --prefix web install
npm --prefix web run dev
```

服务默认监听 `:8080`。`/healthz` 表示进程存活，`/readyz` 表示数据库与迁移完成。首次设置 `CHATOPS_SECURITY_BOOTSTRAP_ADMIN_TOKEN` 时会创建唯一的管理员 Token；请妥善保存它，数据库只保存 Token 的指纹和 Argon2id 验证值。

## 部署

修改 `deployments/kubernetes/deployment.yaml` 中的镜像地址和公开 URL，创建包含 `.env.example` 所列键的 `chatops-deploy-secrets`，然后应用：

```powershell
kubectl apply -f deployments/kubernetes/namespace.yaml
kubectl apply -f deployments/kubernetes/serviceaccount-rbac.yaml
kubectl apply -f deployments/kubernetes/deployment.yaml
kubectl apply -f deployments/kubernetes/service.yaml
```

集群侧 RBAC 需要按每个被管理命名空间部署；示例 `Role` 仅用于演示，不应跨命名空间授予更宽权限。

## GitHub 和 GHCR

将此仓库推送到 GitHub 后，Pull Request 和主分支提交会运行 Go/前端检查、测试和镜像构建。推送严格格式的 tag，例如 `v0.1.0`，会发布：

```text
ghcr.io/<GitHub owner>/chatops-deploy:v0.1.0
ghcr.io/<GitHub owner>/chatops-deploy:latest
```

发布工作流会生成 SBOM、provenance 并通过 GitHub OIDC 执行 keyless Cosign 签名。首次推送前，请在 GitHub 仓库设置中允许 Actions 写入 Packages。

## 安全边界

- 不允许自由指定 Kubernetes 资源；应用环境必须预注册。
- 不记录 kubeconfig、密钥、Token 明文或飞书凭据。
- 生产操作拒绝请求人自批。
- 执行器仅更新指定 Deployment 的指定容器，镜像必须匹配注册仓库前缀。
- 运行生产环境前，请通过 Ingress、NetworkPolicy 和 Secret 管理器限制 API、指标及数据库访问。
