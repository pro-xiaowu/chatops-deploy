package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"chatops-deploy/internal/adapter/feishu"
	"chatops-deploy/internal/application"
	"chatops-deploy/internal/auth"
	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/store/postgres"
	"chatops-deploy/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type API struct {
	app               *application.Service
	store             *postgres.Store
	tokens            *auth.Service
	feishu            *feishu.Client
	verificationToken string
	encryptKey        string
	baseURL           string
	cookieSecure      bool
}

func NewAPI(app *application.Service, store *postgres.Store, tokens *auth.Service, client *feishu.Client, verification, encryptKey, baseURL string, cookieSecure bool) *API {
	return &API{app: app, store: store, tokens: tokens, feishu: client, verificationToken: verification, encryptKey: encryptKey, baseURL: strings.TrimRight(baseURL, "/"), cookieSecure: cookieSecure}
}
func (a *API) Register(r *gin.RouterGroup) {
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
	state := uuid.NewString()
	c.SetCookie("chatops_oauth_state", state, 300, "/", "", a.cookieSecure, true)
	redirect := a.baseURL + "/auth/feishu/callback"
	c.Redirect(http.StatusFound, a.feishu.OAuthURL(redirect, state))
}
func (a *API) OAuthCallback(c *gin.Context) {
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
	c.SetCookie("chatops_session", session.Cookie, int((8 * time.Hour).Seconds()), "/", "", a.cookieSecure, true)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("chatops_csrf", session.CSRF, int((8 * time.Hour).Seconds()), "/", "", a.cookieSecure, false)
	c.Redirect(http.StatusFound, "/")
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

var _ = middleware.Principal
