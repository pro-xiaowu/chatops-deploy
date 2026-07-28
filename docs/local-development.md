# Linux 本地开发

## 环境要求

本地部署和验证只支持 Linux。安装 Docker Engine、Docker Compose v2、`curl`，并确保本机的 `5432` 和 `8080` 端口可用。

## 启动与登录

```bash
docker compose up --build -d
docker compose ps
curl --fail http://localhost:8080/healthz
curl --fail http://localhost:8080/readyz
```

访问 `http://localhost:8080`，点击“本地开发登录”。本地管理员通过正常 Session cookie、CSRF 和 RBAC 路径访问控制台，不使用硬编码 Bearer Token。Compose 中的数据库口令、主密钥和开发登录开关仅用于本地开发，不可复制到生产环境。

查看日志和停止服务：

```bash
docker compose logs -f app
docker compose down
```

清空 PostgreSQL 数据不可恢复，只有在明确需要重新初始化时执行：

```bash
docker compose down --volumes
```

## 消息平台

`CHATOPS_MESSAGE_PROVIDER` 接受 `web`、`feishu`、`wecom` 或 `dingtalk`。管理员也可以在“系统设置”中选择已配置的平台。任何时刻只有一个平台接收新消息；已创建操作保留自己的来源平台和会话，因此结果通知不会因为后续切换而发错平台。

- `web`：仅 Web 控制台，无第三方凭据，Compose 默认模式。
- `feishu`：消息回调需要 `CHATOPS_FEISHU_APP_ID`、`CHATOPS_FEISHU_APP_SECRET`、`CHATOPS_FEISHU_VERIFICATION_TOKEN` 和 `CHATOPS_FEISHU_ENCRYPT_KEY`。仅使用浏览器 OAuth 时只需 App ID 和 App Secret。
- `wecom`：需要 `CHATOPS_WECOM_CORP_ID`、`CHATOPS_WECOM_AGENT_ID`、`CHATOPS_WECOM_SECRET` 和 `CHATOPS_WECOM_TOKEN`。
- `dingtalk`：需要 `CHATOPS_DINGTALK_CLIENT_ID`、`CHATOPS_DINGTALK_CLIENT_SECRET`、`CHATOPS_DINGTALK_ROBOT_CODE` 和 `CHATOPS_DINGTALK_EVENT_TOKEN`。

浏览器认证和消息投递相互独立。飞书可用于浏览器 OAuth；企业微信和钉钉只提供 ChatOps 回调和通知，不提供浏览器 OAuth。平台密钥只来自环境变量或 Secret，不保存到 PostgreSQL，也不会由能力 API 返回。

企业微信与钉钉的入站端点接收网关标准化后的 JSON 事件。企业微信请求需携带 `timestamp`、`nonce` 和 `msg_signature`，签名为 Token、时间戳、nonce、原始请求体排序拼接后的 SHA-1；钉钉请求需携带 `timestamp` 和 `signature`，签名为 Event Token、时间戳、原始请求体拼接后的 SHA-256。直接使用平台原生加密 XML/事件流时，需要在入口网关完成解密和字段标准化。

管理员在“访问控制”中为本地用户绑定平台 Subject ID；同一用户可以绑定多个平台身份并复用相同 RBAC 权限。

## 验证

```bash
go vet ./...
go test ./...
npm --prefix web ci
npm --prefix web run check
npm --prefix web run lint
npm --prefix web run test -- --run
npm --prefix web run build
bash scripts/compose-smoke.sh
```

PostgreSQL 集成测试使用 `CHATOPS_TEST_DATABASE_URL`。Playwright 需要先执行 `npm --prefix web exec -- playwright install --with-deps chromium`，然后在 Compose 运行期间执行 `npm --prefix web run test:e2e`。GitHub Actions 在 Linux runner 上执行上述后端、前端、镜像和浏览器检查。
