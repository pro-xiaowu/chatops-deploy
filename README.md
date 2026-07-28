# chatops-deploy

在飞书、企业微信、钉钉或 Web 运维控制台中创建 Kubernetes Deployment 的部署、回滚、审批与状态查询操作。服务使用 PostgreSQL 持久化操作状态和审计记录，使用 `client-go` 操作预注册应用，生产环境变更必须经过另一名授权用户审批。

## 功能

- 飞书、企业微信和钉钉 `/deploy`、`/rollback`、`/approve`、`/status` 和 `/help` 命令入口
- 管理员可选择飞书、企业微信、钉钉或仅 Web 控制台消息模式
- 飞书 OAuth 登录和同域 Web 控制台
- 多集群 kubeconfig AES-256-GCM 加密存储
- Deployment 镜像部署、rollout 观察、ReplicaSet revision 回滚
- PostgreSQL 任务队列、审批、审计、outbox 和 SSE 事件
- 管理 API Token 使用 Argon2id 哈希，浏览器写请求使用 CSRF Token
- Docker 多阶段构建、Kubernetes 清单、GitHub CI 与 GHCR 多架构发布

## 本地运行

本项目本地部署只支持 Linux。需要 Docker Engine、Docker Compose v2 和 `curl`。

```bash
docker compose up --build -d
docker compose ps
curl --fail http://localhost:8080/readyz
```

浏览器打开 `http://localhost:8080`，点击“本地开发登录”。Compose 默认使用 `CHATOPS_RUNTIME_ENV=development`、`CHATOPS_DEV_AUTH_ENABLED=true` 和 `CHATOPS_MESSAGE_PROVIDER=web`；这些凭据和开关只用于本地开发。

```bash
docker compose logs -f app
docker compose down
```

需要清空本地数据库时，显式删除数据卷：

```bash
docker compose down --volumes
```

服务默认监听 `:8080`。`/healthz` 表示进程存活，`/readyz` 表示数据库与迁移完成。完整的 Linux 运行、消息平台配置和测试说明见 [docs/local-development.md](docs/local-development.md)。

## 部署

修改 `deployments/kubernetes/deployment.yaml` 中的镜像地址和公开 URL，创建包含 `.env.example` 所列键的 `chatops-deploy-secrets`，然后应用：

```bash
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
- 外部回调必须通过平台验证 Token 或请求签名校验，缺少签名的请求会被拒绝。
- 执行器仅更新指定 Deployment 的指定容器，镜像必须匹配注册仓库前缀。
- 运行生产环境前，请通过 Ingress、NetworkPolicy 和 Secret 管理器限制 API、指标及数据库访问。
