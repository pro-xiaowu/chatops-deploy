# chatops-deploy

[简体中文](README.md) | [English](README.en.md)

`chatops-deploy` 是一个面向 Kubernetes Deployment 的 ChatOps 运维平台。运维人员可以通过 Web 控制台、飞书、企业微信或钉钉发起部署、回滚、审批和状态查询。服务使用 PostgreSQL 保存操作状态、身份映射、审计事件和可靠通知，生产环境变更必须由另一名授权用户审批。

> 本项目的本地部署和开发环境仅支持 Linux。

## 主要功能

- `/deploy`、`/rollback`、`/approve`、`/status` 和 `/help` ChatOps 命令
- 飞书、企业微信、钉钉或仅 Web 控制台的消息模式切换
- 独立的浏览器认证与消息平台身份映射，同一用户可以绑定多个平台身份
- 多集群 kubeconfig AES-256-GCM 加密存储
- Deployment 镜像更新、rollout 观察和 ReplicaSet revision 回滚
- PostgreSQL 任务队列、审批、审计、事务 outbox、领取租约和 SSE 事件
- 基于 RBAC 的应用权限、Argon2id API Token 和浏览器 CSRF 防护
- Linux Docker 多阶段镜像、Docker Compose、Kubernetes 清单和 GitHub Actions

## Linux 快速启动

需要 Docker Engine、Docker Compose v2 和 `curl`，并确保本机的 `5432` 与 `8080` 端口可用。

```bash
docker compose up --build -d --wait
docker compose ps
curl --fail http://localhost:8080/readyz
```

打开 http://localhost:8080 并点击“本地开发登录”。Compose 默认启用仅限开发环境的 Session 登录，并使用 `web` 消息模式，不需要任何第三方平台凭据。

```bash
docker compose logs -f app
docker compose down
```

需要重置本地数据库时，显式删除数据卷。该操作不可恢复：

```bash
docker compose down --volumes
```

`/healthz` 表示进程存活，`/readyz` 表示数据库连接和迁移均已就绪。更完整的配置说明见 [Linux 本地开发文档](docs/local-development.md)。

## 消息平台

管理员可以在“系统设置”中选择一个已配置且健康检查通过的平台。切换平台不会改变浏览器登录方式；历史操作始终使用创建该操作时记录的来源平台发送结果。

| 模式 | 入站命令与审批 | 出站通知 | 浏览器登录 |
| --- | --- | --- | --- |
| Web | Web 控制台 | Web 控制台 | 本地开发 Session 或已配置的飞书 OAuth |
| 飞书 | 签名与 Verification Token 校验，支持加密事件 | 文本与交互卡片 | 支持 OAuth |
| 企业微信 | 签名后的标准化 JSON 回调 | 应用消息 | 不提供 |
| 钉钉 | 签名后的标准化 JSON 回调 | 机器人单聊消息 | 不提供 |

企业微信或钉钉直接使用原生加密 XML/事件流时，需要入口网关先完成解密和字段标准化。所需环境变量和签名契约记录在 [Linux 本地开发文档](docs/local-development.md)。

## 测试

后端测试包含 PostgreSQL 迁移与存储、状态机、消息平台验签、Kubernetes fake client、HTTP 认证和 worker outbox。前端包含类型检查、lint、Vitest，以及在 GitHub Actions 中运行的桌面与移动端 Playwright 测试。

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

PostgreSQL 集成测试通过 `CHATOPS_TEST_DATABASE_URL` 指定测试数据库。CI 会自动启动独立的 PostgreSQL 17 服务。

## Kubernetes 部署

修改 `deployments/kubernetes/deployment.yaml` 中的镜像地址和公开 URL，创建包含 `.env.example` 所列键的 `chatops-deploy-secrets`，然后应用清单：

```bash
kubectl apply -f deployments/kubernetes/namespace.yaml
kubectl apply -f deployments/kubernetes/serviceaccount-rbac.yaml
kubectl apply -f deployments/kubernetes/deployment.yaml
kubectl apply -f deployments/kubernetes/service.yaml
```

集群侧 RBAC 应按每个受管命名空间部署。示例 `Role` 仅用于演示，不应跨命名空间授予更宽权限。

## GitHub 和 GHCR

Pull Request、`main` 和 `feature/**` 分支提交会运行 Go、前端、镜像与 Compose 浏览器烟测。推送严格的语义版本 tag，例如 `v0.1.0`，会发布：

```text
ghcr.io/<GitHub owner>/chatops-deploy:v0.1.0
ghcr.io/<GitHub owner>/chatops-deploy:latest
```

发布工作流会生成 SBOM 和 provenance，并使用 GitHub OIDC 执行 keyless Cosign 签名。

## 安全边界

- 应用和环境必须预注册，用户不能自由提交 Kubernetes 资源。
- kubeconfig、平台密钥和 Token 不会以明文写入日志。
- 生产操作禁止请求人自批。
- 缺少签名或验证失败的外部回调会被拒绝。
- worker 只更新指定 Deployment 的指定容器，镜像必须匹配注册仓库前缀。
- 生产环境应通过 Ingress、NetworkPolicy 和 Secret 管理器限制 API、指标及数据库访问。
