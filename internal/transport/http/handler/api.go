package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"chatops-deploy/internal/adapter/feishu"
	"chatops-deploy/internal/application"
	"chatops-deploy/internal/auth"
	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/messaging"
	"chatops-deploy/internal/store/postgres"
	"chatops-deploy/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (a *API) Capabilities(c *gin.Context) { a.capabilities(c) }
func (a *API) DevLogin(c *gin.Context)     { a.devLogin(c) }

func (a *API) Webhook(c *gin.Context) {
	if a.registry == nil {
		c.Status(http.StatusNotFound)
		return
	}
	requested := domain.MessageProvider(c.Param("provider"))
	requestContext := c.Request.Context()
	active, err := a.registry.Active(requestContext)
	if err != nil || active.Name() == domain.MessageProviderWeb || requested != active.Name() {
		c.Status(http.StatusNotFound)
		return
	}
	body, err := c.GetRawData()
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	incoming, err := active.Decode(requestContext, messaging.Request{Method: c.Request.Method, Body: body, Query: map[string]string{
		"msg_signature": c.Query("msg_signature"),
		"signature":     c.Query("signature"),
		"timestamp":     c.Query("timestamp"),
		"nonce":         c.Query("nonce"),
	}, Headers: map[string]string{
		"X-Lark-Request-Timestamp": c.GetHeader("X-Lark-Request-Timestamp"),
		"X-Lark-Request-Nonce":     c.GetHeader("X-Lark-Request-Nonce"),
		"X-Lark-Signature":         c.GetHeader("X-Lark-Signature"),
		"X-WeCom-Token":            c.GetHeader("X-WeCom-Token"),
		"X-DingTalk-Token":         c.GetHeader("X-DingTalk-Token"),
		"X-WeCom-Signature":        c.GetHeader("X-WeCom-Signature"),
		"X-DingTalk-Signature":     c.GetHeader("X-DingTalk-Signature"),
	}})
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "invalid_webhook", "message": "provider callback validation failed"}})
		return
	}
	if incoming.Challenge != "" {
		c.JSON(http.StatusOK, gin.H{"challenge": incoming.Challenge})
		return
	}
	if incoming.OperationID != "" {
		a.handleIncomingApproval(requestContext, incoming)
		c.Status(http.StatusOK)
		return
	}
	if incoming.Command == "" {
		c.Status(http.StatusOK)
		return
	}
	user, err := a.store.FindUserByIdentity(requestContext, incoming.Provider, incoming.SubjectID)
	if err != nil {
		_ = active.Send(context.Background(), messaging.Notification{Destination: incoming.ConversationID, Text: "你还没有 ChatOps 权限，请联系管理员。"})
		c.Status(http.StatusOK)
		return
	}
	principal, err := a.store.PrincipalForUser(requestContext, user.ID)
	if err != nil {
		c.Status(http.StatusOK)
		return
	}
	message := a.handleIncomingCommand(requestContext, incoming, user, principal)
	if message != "" {
		_ = active.Send(requestContext, messaging.Notification{Destination: incoming.ConversationID, Text: message})
	}
	c.Status(http.StatusOK)
}

func (a *API) handleIncomingApproval(ctx context.Context, incoming messaging.Incoming) {
	operationID, err := uuid.Parse(incoming.OperationID)
	if err != nil {
		return
	}
	approver, err := a.store.FindUserByIdentity(ctx, incoming.Provider, incoming.SubjectID)
	if err != nil {
		return
	}
	principal, err := a.store.PrincipalForUser(ctx, approver.ID)
	if err != nil {
		return
	}
	operation, err := a.store.GetOperation(ctx, operationID)
	if err != nil || !principal.Allows(domain.ActionApprove, operation.ApplicationID) {
		return
	}
	_ = a.app.Decide(ctx, operationID, approver.ID, incoming.Decision, "")
}

func (a *API) handleIncomingCommand(ctx context.Context, incoming messaging.Incoming, user domain.User, principal domain.Principal) string {
	origin := domain.MessageOrigin{Provider: incoming.Provider, ConversationID: incoming.ConversationID, EventID: incoming.EventID}
	idempotencyKey := string(incoming.Provider) + ":" + incoming.EventID
	switch incoming.Command {
	case "help":
		return feishu.HelpText()
	case "deploy":
		if len(incoming.Arguments) != 2 {
			return "用法：/deploy <environment-id> <image>"
		}
		environmentID, err := uuid.Parse(incoming.Arguments[0])
		if err != nil {
			return "环境 ID 格式无效。"
		}
		environment, err := a.store.GetEnvironment(ctx, environmentID)
		if err != nil || !principal.Allows(domain.ActionDeploy, environment.ApplicationID) {
			return "你没有该应用的部署权限。"
		}
		operation, err := a.app.RequestOperationWithOrigin(ctx, user.ID, domain.OperationDeploy, environmentID, incoming.Arguments[1], 0, idempotencyKey, origin)
		if err != nil {
			return "创建部署请求失败：" + err.Error()
		}
		if operation.Status == domain.StatusPendingApproval {
			if provider, ok := a.registry.Provider(incoming.Provider); ok {
				_ = provider.Send(ctx, messaging.Notification{OperationID: operation.ID.String(), Destination: incoming.ConversationID, Text: "生产部署待审批：" + operation.ID.String(), ApprovalCard: feishu.ApprovalCard(operation.ID.String(), "部署", incoming.Arguments[1])})
			}
		}
		return "部署请求已创建，请等待执行或审批。"
	case "rollback":
		if len(incoming.Arguments) != 2 {
			return "用法：/rollback <environment-id> <revision>"
		}
		environmentID, parseErr := uuid.Parse(incoming.Arguments[0])
		revision, revisionErr := strconv.ParseInt(incoming.Arguments[1], 10, 64)
		if parseErr != nil || revisionErr != nil {
			return "环境 ID 或 revision 格式无效。"
		}
		environment, err := a.store.GetEnvironment(ctx, environmentID)
		if err != nil || !principal.Allows(domain.ActionRollback, environment.ApplicationID) {
			return "你没有该应用的回滚权限。"
		}
		operation, err := a.app.RequestOperationWithOrigin(ctx, user.ID, domain.OperationRollback, environmentID, "", revision, idempotencyKey, origin)
		if err != nil {
			return "创建回滚请求失败：" + err.Error()
		}
		if operation.Status == domain.StatusPendingApproval {
			if provider, ok := a.registry.Provider(incoming.Provider); ok {
				_ = provider.Send(ctx, messaging.Notification{OperationID: operation.ID.String(), Destination: incoming.ConversationID, Text: "生产回滚待审批：" + operation.ID.String(), ApprovalCard: feishu.ApprovalCard(operation.ID.String(), "回滚", fmt.Sprintf("revision %d", revision))})
			}
		}
		return "回滚请求已创建，请等待执行或审批。"
	case "approve":
		if len(incoming.Arguments) != 2 {
			return "用法：/approve <operation-id> <approved|rejected>"
		}
		operationID, err := uuid.Parse(incoming.Arguments[0])
		if err != nil || (incoming.Arguments[1] != "approved" && incoming.Arguments[1] != "rejected") {
			return "操作单 ID 或审批决定格式无效。"
		}
		operation, err := a.store.GetOperation(ctx, operationID)
		if err != nil || !principal.Allows(domain.ActionApprove, operation.ApplicationID) {
			return "你没有该操作单的审批权限。"
		}
		if err := a.app.Decide(ctx, operationID, user.ID, incoming.Arguments[1], ""); err != nil {
			return "审批失败：" + err.Error()
		}
		return "审批已提交。"
	case "status":
		if len(incoming.Arguments) != 1 {
			return "用法：/status <environment-id>"
		}
		environmentID, err := uuid.Parse(incoming.Arguments[0])
		if err != nil {
			return "环境 ID 格式无效。"
		}
		environment, err := a.store.GetEnvironment(ctx, environmentID)
		if err != nil {
			return "未找到该环境。"
		}
		if !principal.Allows(domain.ActionView, environment.ApplicationID) {
			return "你没有该应用的查看权限。"
		}
		return "环境 " + environment.Name + "：" + environment.Namespace + "/" + environment.Deployment
	default:
		return feishu.HelpText()
	}
}

type API struct {
	app                *application.Service
	store              *postgres.Store
	tokens             *auth.Service
	feishu             *feishu.Client
	verificationToken  string
	encryptKey         string
	baseURL            string
	cookieSecure       bool
	registry           *messaging.Registry
	runtimeEnvironment string
	devAuthEnabled     bool
}

func NewAPI(app *application.Service, store *postgres.Store, tokens *auth.Service, client *feishu.Client, verification, encryptKey, baseURL string, cookieSecure bool) *API {
	return NewAPIWithOptions(app, store, tokens, client, verification, encryptKey, baseURL, cookieSecure, nil, "production", false)
}

func NewAPIWithOptions(app *application.Service, store *postgres.Store, tokens *auth.Service, client *feishu.Client, verification, encryptKey, baseURL string, cookieSecure bool, registry *messaging.Registry, runtimeEnvironment string, devAuthEnabled bool) *API {
	return &API{app: app, store: store, tokens: tokens, feishu: client, verificationToken: verification, encryptKey: encryptKey, baseURL: strings.TrimRight(baseURL, "/"), cookieSecure: cookieSecure, registry: registry, runtimeEnvironment: runtimeEnvironment, devAuthEnabled: devAuthEnabled}
}
func (a *API) Register(r *gin.RouterGroup) {
	r.GET("/me", a.me)
	r.GET("/clusters", a.listClusters)
	r.POST("/clusters", a.createCluster)
	r.GET("/applications", a.listApplications)
	r.POST("/applications", a.createApplication)
	r.GET("/environments", a.listEnvironments)
	r.POST("/environments", a.createEnvironment)
	r.GET("/operations", a.listOperations)
	r.POST("/operations", a.requestOperation)
	r.GET("/operations/:id", a.getOperation)
	r.POST("/operations/:id/approval", a.approve)
	r.GET("/users", a.listUsers)
	r.POST("/users", a.createUser)
	r.GET("/roles", a.listRoles)
	r.POST("/roles", a.createRole)
	r.POST("/roles/:id/users/:user_id", a.addUserRole)
	r.GET("/roles/:id/permissions", a.listPermissions)
	r.POST("/roles/:id/permissions", a.grantPermission)
	r.POST("/api-tokens", a.issueToken)
	r.DELETE("/api-tokens/:id", a.revokeToken)
	r.GET("/settings/message-provider", a.getMessageProvider)
	r.PUT("/settings/message-provider", a.setMessageProvider)
	r.POST("/settings/message-provider/:provider/check", a.checkMessageProvider)
	r.GET("/users/:id/identities", a.listIdentities)
	r.POST("/users/:id/identities", a.createIdentity)
}
func decode(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": err.Error()}})
		return false
	}
	return true
}
func id(v string) (uuid.UUID, bool) { x, err := uuid.Parse(v); return x, err == nil }
func (a *API) listClusters(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	rows, err := a.store.ListClusters(c)
	respond(c, rows, err)
}
func (a *API) createCluster(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	var in application.CreateClusterInput
	if !decode(c, &in) {
		return
	}
	out, err := a.app.CreateCluster(c, in)
	respondStatus(c, http.StatusCreated, out, err)
}
func (a *API) listApplications(c *gin.Context) {
	rows, err := a.store.ListApplications(c)
	if err == nil {
		principal, _ := middleware.Principal(c)
		filtered := rows[:0]
		for _, row := range rows {
			if canView(principal, row.ID) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	respond(c, rows, err)
}
func (a *API) createApplication(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	var in struct{ Name, Description string }
	if !decode(c, &in) {
		return
	}
	out, err := a.app.CreateApplication(c, in.Name, in.Description)
	respondStatus(c, http.StatusCreated, out, err)
}
func (a *API) listEnvironments(c *gin.Context) {
	var appID *uuid.UUID
	if raw := c.Query("application_id"); raw != "" {
		v, ok := id(raw)
		if !ok {
			c.JSON(400, gin.H{"error": gin.H{"code": "invalid_request", "message": "invalid application_id"}})
			return
		}
		appID = &v
	}
	rows, err := a.store.ListEnvironments(c, appID)
	if err == nil {
		principal, _ := middleware.Principal(c)
		filtered := rows[:0]
		for _, row := range rows {
			if canView(principal, row.ApplicationID) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	respond(c, rows, err)
}
func (a *API) createEnvironment(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	var in domain.AppEnvironment
	if !decode(c, &in) {
		return
	}
	out, err := a.app.CreateEnvironment(c, in)
	respondStatus(c, http.StatusCreated, out, err)
}
func (a *API) listOperations(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	rows, err := a.store.ListOperations(c, limit)
	if err == nil {
		principal, _ := middleware.Principal(c)
		filtered := rows[:0]
		for _, row := range rows {
			if canView(principal, row.ApplicationID) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	respond(c, rows, err)
}
func (a *API) getOperation(c *gin.Context) {
	opID, ok := id(c.Param("id"))
	if !ok {
		c.Status(400)
		return
	}
	row, err := a.store.GetOperation(c, opID)
	if err == nil {
		principal, _ := middleware.Principal(c)
		if !canView(principal, row.ApplicationID) {
			err = domain.ErrForbidden
		}
	}
	respond(c, row, err)
}
func (a *API) requestOperation(c *gin.Context) {
	var in struct {
		Kind          domain.OperationKind `json:"kind"`
		EnvironmentID uuid.UUID            `json:"environment_id"`
		RequesterID   uuid.UUID            `json:"requester_id"`
		Image         string               `json:"image"`
		Revision      int64                `json:"revision"`
	}
	if !decode(c, &in) {
		return
	}
	principal, ok := middleware.Principal(c)
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}
	environment, err := a.store.GetEnvironment(c, in.EnvironmentID)
	if err != nil {
		respond(c, nil, err)
		return
	}
	action := domain.ActionDeploy
	if in.Kind == domain.OperationRollback {
		action = domain.ActionRollback
	}
	if !principal.Allows(action, environment.ApplicationID) {
		respond(c, nil, domain.ErrForbidden)
		return
	}
	if principal.UserID != nil {
		in.RequesterID = *principal.UserID
	}
	if in.RequesterID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "requester identity is required"}})
		return
	}
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		key = uuid.NewString()
	}
	out, err := a.app.RequestOperation(c, in.RequesterID, in.Kind, in.EnvironmentID, in.Image, in.Revision, key)
	respondStatus(c, http.StatusCreated, out, err)
}
func (a *API) approve(c *gin.Context) {
	operationID, ok := id(c.Param("id"))
	if !ok {
		c.Status(400)
		return
	}
	var in struct {
		ApproverID        uuid.UUID `json:"approver_id"`
		Decision, Comment string
	}
	if !decode(c, &in) {
		return
	}
	principal, ok := middleware.Principal(c)
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}
	operation, err := a.store.GetOperation(c, operationID)
	if err != nil {
		respond(c, nil, err)
		return
	}
	if !principal.Allows(domain.ActionApprove, operation.ApplicationID) {
		respond(c, nil, domain.ErrForbidden)
		return
	}
	if principal.UserID != nil {
		in.ApproverID = *principal.UserID
	}
	if in.ApproverID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "approver identity is required"}})
		return
	}
	respond(c, nil, a.app.Decide(c, operationID, in.ApproverID, in.Decision, in.Comment))
}
func (a *API) listUsers(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	rows, err := a.store.ListUsers(c)
	respond(c, rows, err)
}
func (a *API) createUser(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	var in struct {
		OpenID string `json:"feishu_open_id"`
		Name   string `json:"display_name"`
	}
	if !decode(c, &in) {
		return
	}
	out, err := a.store.CreateUser(c, domain.User{FeishuOpenID: in.OpenID, DisplayName: in.Name, Enabled: true})
	respondStatus(c, http.StatusCreated, out, err)
}
func (a *API) listRoles(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	rows, err := a.store.ListRoles(c)
	respond(c, rows, err)
}
func (a *API) createRole(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	var in domain.Role
	if !decode(c, &in) {
		return
	}
	out, err := a.store.CreateRole(c, in)
	respondStatus(c, http.StatusCreated, out, err)
}
func (a *API) addUserRole(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	roleID, ok1 := id(c.Param("id"))
	userID, ok2 := id(c.Param("user_id"))
	if !ok1 || !ok2 {
		c.Status(http.StatusBadRequest)
		return
	}
	respond(c, nil, a.store.AddUserRole(c, userID, roleID))
}
func (a *API) listPermissions(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	roleID, ok := id(c.Param("id"))
	if !ok {
		c.Status(http.StatusBadRequest)
		return
	}
	rows, err := a.store.ListPermissions(c, roleID)
	respond(c, rows, err)
}
func (a *API) grantPermission(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	roleID, ok := id(c.Param("id"))
	if !ok {
		c.Status(http.StatusBadRequest)
		return
	}
	var in domain.Permission
	if !decode(c, &in) {
		return
	}
	in.RoleID = roleID
	out, err := a.store.GrantPermission(c, in)
	respondStatus(c, http.StatusCreated, out, err)
}
func (a *API) issueToken(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	var in struct {
		Name      string     `json:"name"`
		RoleID    uuid.UUID  `json:"role_id"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if !decode(c, &in) {
		return
	}
	out, err := a.tokens.Issue(c, in.Name, in.RoleID, in.ExpiresAt)
	respondStatus(c, http.StatusCreated, out, err)
}
func (a *API) revokeToken(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	tokenID, ok := id(c.Param("id"))
	if !ok {
		c.Status(400)
		return
	}
	respond(c, nil, a.tokens.Revoke(c, tokenID))
}

func (a *API) capabilities(c *gin.Context) {
	providers := []messaging.Capabilities{}
	feishuLogin := a.feishu != nil && a.feishu.Configured()
	if a.registry != nil {
		providers = a.registry.Capabilities(c.Request.Context())
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"dev_login": a.runtimeEnvironment == "development" && a.devAuthEnabled, "feishu_login": feishuLogin, "message_providers": providers}})
}

func (a *API) devLogin(c *gin.Context) {
	if a.runtimeEnvironment != "development" || !a.devAuthEnabled || a.store == nil || a.tokens == nil {
		c.Status(http.StatusNotFound)
		return
	}
	requestContext := c.Request.Context()
	user, err := a.store.EnsureDevelopmentAdmin(requestContext)
	if err != nil {
		respond(c, nil, err)
		return
	}
	session, err := a.tokens.CreateSession(requestContext, user, 8*time.Hour)
	if err != nil {
		respond(c, nil, err)
		return
	}
	setSessionCookies(c, session, a.cookieSecure)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"user": user}})
}

func (a *API) me(c *gin.Context) {
	principal, ok := middleware.Principal(c)
	if !ok || principal.UserID == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "session authentication required"}})
		return
	}
	user, err := a.store.GetUser(c.Request.Context(), *principal.UserID)
	respond(c, user, err)
}

func (a *API) getMessageProvider(c *gin.Context) {
	if a.registry == nil {
		respond(c, nil, errors.New("message providers are unavailable"))
		return
	}
	requestContext := c.Request.Context()
	active, err := a.registry.Active(requestContext)
	if err != nil {
		respond(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"provider": active.Name(), "providers": a.registry.Capabilities(requestContext)}})
}

func (a *API) setMessageProvider(c *gin.Context) {
	if !requireAdmin(c) || a.registry == nil {
		return
	}
	var input struct {
		Provider domain.MessageProvider `json:"provider"`
	}
	if !decode(c, &input) {
		return
	}
	if err := a.registry.Select(c.Request.Context(), input.Provider); err != nil {
		respond(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"provider": input.Provider}})
}

func (a *API) checkMessageProvider(c *gin.Context) {
	if !requireAdmin(c) || a.registry == nil {
		return
	}
	providerName := domain.MessageProvider(c.Param("provider"))
	capabilities, err := a.registry.Check(c.Request.Context(), providerName)
	respond(c, capabilities, err)
}

func (a *API) listIdentities(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	userID, ok := id(c.Param("id"))
	if !ok {
		c.Status(http.StatusBadRequest)
		return
	}
	rows, err := a.store.ListMessageProviderIdentities(c.Request.Context(), userID)
	respond(c, rows, err)
}

func (a *API) createIdentity(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	userID, ok := id(c.Param("id"))
	if !ok {
		c.Status(http.StatusBadRequest)
		return
	}
	var input struct {
		Provider    domain.MessageProvider `json:"provider"`
		SubjectID   string                 `json:"subject_id"`
		DisplayName string                 `json:"display_name"`
	}
	if !decode(c, &input) {
		return
	}
	err := a.store.UpsertExternalIdentity(c.Request.Context(), domain.ExternalIdentity{UserID: userID, Provider: input.Provider, SubjectID: input.SubjectID, DisplayName: input.DisplayName})
	respond(c, nil, err)
}
func requireAdmin(c *gin.Context) bool {
	principal, ok := middleware.Principal(c)
	if !ok || !principal.Allows(domain.ActionAdmin, uuid.Nil) {
		respond(c, nil, domain.ErrForbidden)
		return false
	}
	return true
}
func canView(principal domain.Principal, appID uuid.UUID) bool {
	return principal.Allows(domain.ActionAdmin, appID) || principal.Allows(domain.ActionView, appID) || principal.Allows(domain.ActionDeploy, appID) || principal.Allows(domain.ActionRollback, appID) || principal.Allows(domain.ActionApprove, appID)
}
func respond(c *gin.Context, data any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"data": data})
		return
	}
	status := http.StatusInternalServerError
	code := "internal_error"
	if err == domain.ErrNotFound {
		status = 404
		code = "not_found"
	}
	if err == domain.ErrConflict {
		status = 409
		code = "conflict"
	}
	if err == domain.ErrForbidden {
		status = 403
		code = "forbidden"
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": err.Error(), "request_id": c.GetString("request_id")}})
}
func respondStatus(c *gin.Context, status int, data any, err error) {
	if err != nil {
		respond(c, nil, err)
		return
	}
	c.JSON(status, gin.H{"data": data})
}

func (a *API) FeishuWebhook(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	if !feishu.VerifySignature(c.GetHeader("X-Lark-Request-Timestamp"), c.GetHeader("X-Lark-Request-Nonce"), a.encryptKey, c.GetHeader("X-Lark-Signature"), body) {
		c.Status(http.StatusUnauthorized)
		return
	}
	body, err = feishu.DecryptEvent(body, a.encryptKey)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	event, err := feishu.DecodeEvent(body, a.verificationToken)
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	if event.Event.Action.Value["operation_id"] != "" {
		a.handleCardApproval(c, event)
		c.Status(http.StatusOK)
		return
	}
	if event.Challenge != "" {
		c.JSON(http.StatusOK, gin.H{"challenge": event.Challenge})
		return
	}
	command, args := feishu.Command(event.Event.Message.Content)
	if command == "" {
		c.Status(http.StatusOK)
		return
	}
	user, err := a.store.FindUserByOpenID(c, event.Event.Sender.SenderID.OpenID)
	if err != nil {
		_ = a.feishu.SendText(context.Background(), event.Event.Message.ChatID, "你还没有 ChatOps 权限，请联系管理员。")
		c.Status(http.StatusOK)
		return
	}
	principal, err := a.store.PrincipalForUser(c, user.ID)
	if err != nil {
		c.Status(http.StatusOK)
		return
	}

	message := a.handleCommand(c, event, user, principal, command, args)
	if message != "" {
		_ = a.feishu.SendText(c, event.Event.Message.ChatID, message)
	}
	c.Status(http.StatusOK)
}

func (a *API) handleCardApproval(ctx context.Context, event feishu.Event) {
	operationID, err := uuid.Parse(event.Event.Action.Value["operation_id"])
	if err != nil {
		return
	}
	approver, err := a.store.FindUserByOpenID(ctx, event.Event.Operator.OpenID)
	if err != nil {
		return
	}
	principal, err := a.store.PrincipalForUser(ctx, approver.ID)
	if err != nil {
		return
	}
	operation, err := a.store.GetOperation(ctx, operationID)
	if err != nil || !principal.Allows(domain.ActionApprove, operation.ApplicationID) {
		return
	}
	_ = a.app.Decide(ctx, operationID, approver.ID, event.Event.Action.Value["decision"], "")
}

func (a *API) handleCommand(ctx context.Context, event feishu.Event, user domain.User, principal domain.Principal, command string, args []string) string {
	switch command {
	case "help":
		return feishu.HelpText()
	case "deploy":
		if len(args) != 2 {
			return "用法：/deploy <environment-id> <image>"
		}
		envID, err := uuid.Parse(args[0])
		if err != nil {
			return "环境 ID 格式无效。"
		}
		environment, err := a.store.GetEnvironment(ctx, envID)
		if err != nil || !principal.Allows(domain.ActionDeploy, environment.ApplicationID) {
			return "你没有该应用的部署权限。"
		}
		operation, err := a.app.RequestOperation(ctx, user.ID, domain.OperationDeploy, envID, args[1], 0, event.Header.EventID)
		if err != nil {
			return "创建部署请求失败：" + err.Error()
		}
		if operation.Status == domain.StatusPendingApproval {
			_ = a.feishu.SendCard(ctx, event.Event.Message.ChatID, feishu.ApprovalCard(operation.ID.String(), "部署", args[1]))
		}
		return "部署请求已创建，请等待执行或审批。"
	case "rollback":
		if len(args) != 2 {
			return "用法：/rollback <environment-id> <revision>"
		}
		envID, err := uuid.Parse(args[0])
		revision, revisionErr := strconv.ParseInt(args[1], 10, 64)
		if err != nil || revisionErr != nil {
			return "环境 ID 或 revision 格式无效。"
		}
		environment, err := a.store.GetEnvironment(ctx, envID)
		if err != nil || !principal.Allows(domain.ActionRollback, environment.ApplicationID) {
			return "你没有该应用的回滚权限。"
		}
		operation, err := a.app.RequestOperation(ctx, user.ID, domain.OperationRollback, envID, "", revision, event.Header.EventID)
		if err != nil {
			return "创建回滚请求失败：" + err.Error()
		}
		if operation.Status == domain.StatusPendingApproval {
			_ = a.feishu.SendCard(ctx, event.Event.Message.ChatID, feishu.ApprovalCard(operation.ID.String(), "回滚", fmt.Sprintf("revision %d", revision)))
		}
		return "回滚请求已创建，请等待执行或审批。"
	case "status":
		if len(args) != 1 {
			return "用法：/status <environment-id>"
		}
		envID, err := uuid.Parse(args[0])
		if err != nil {
			return "环境 ID 格式无效。"
		}
		env, err := a.store.GetEnvironment(ctx, envID)
		if err != nil {
			return "未找到该环境。"
		}
		if !principal.Allows(domain.ActionView, env.ApplicationID) {
			return "你没有该应用的查看权限。"
		}
		return "环境 " + env.Name + "：" + env.Namespace + "/" + env.Deployment
	default:
		return feishu.HelpText()
	}
}
func (a *API) Login(c *gin.Context) {
	if !a.feishuLoginAvailable(c) {
		c.Status(http.StatusNotFound)
		return
	}
	state := uuid.NewString()
	c.SetCookie("chatops_oauth_state", state, 300, "/", "", a.cookieSecure, true)
	redirect := a.baseURL + "/auth/feishu/callback"
	c.Redirect(http.StatusFound, a.feishu.OAuthURL(redirect, state))
}
func (a *API) OAuthCallback(c *gin.Context) {
	if !a.feishuLoginAvailable(c) {
		c.Status(http.StatusNotFound)
		return
	}
	if c.Query("state") == "" || c.Query("state") != cookie(c, "chatops_oauth_state") {
		c.Redirect(http.StatusFound, "/?auth=failed")
		return
	}
	openID, err := a.feishu.ExchangeCode(c, c.Query("code"), a.baseURL+"/auth/feishu/callback")
	if err != nil {
		c.Redirect(http.StatusFound, "/?auth=failed")
		return
	}
	user, err := a.store.FindUserByOpenID(c, openID)
	if err != nil {
		c.Redirect(http.StatusFound, "/?auth=forbidden")
		return
	}
	session, err := a.tokens.CreateSession(c, user, 8*time.Hour)
	if err != nil {
		c.Redirect(http.StatusFound, "/?auth=failed")
		return
	}
	setSessionCookies(c, session, a.cookieSecure)
	c.Redirect(http.StatusFound, "/")
}

func (a *API) feishuLoginAvailable(_ context.Context) bool {
	return a.feishu != nil && a.feishu.Configured() && a.store != nil && a.tokens != nil
}
func (a *API) Events(c *gin.Context) {
	after, _ := strconv.ParseInt(c.GetHeader("Last-Event-ID"), 10, 64)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		events, err := a.store.RecentEvents(c, after, 100)
		if err != nil {
			return
		}
		for _, event := range events {
			c.SSEvent(event.Kind, gin.H{"id": event.ID, "operation_id": event.OperationID, "payload": event.Payload})
			after = event.ID
		}
		c.Writer.Flush()
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
func cookie(c *gin.Context, name string) string { v, _ := c.Cookie(name); return v }

func setSessionCookies(c *gin.Context, session auth.Session, secure bool) {
	maxAge := int((8 * time.Hour).Seconds())
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("chatops_session", session.Cookie, maxAge, "/", "", secure, true)
	c.SetCookie("chatops_csrf", session.CSRF, maxAge, "/", "", secure, false)
}

var _ = middleware.Principal
