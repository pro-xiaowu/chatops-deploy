package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"chatops-deploy/internal/adapter/dingtalk"
	messagingweb "chatops-deploy/internal/adapter/messaging"
	"chatops-deploy/internal/auth"
	"chatops-deploy/internal/config"
	"chatops-deploy/internal/messaging"
	storepostgres "chatops-deploy/internal/store/postgres"
	transporthttp "chatops-deploy/internal/transport/http"
	"chatops-deploy/internal/transport/http/handler"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDevLoginIsUnavailableInProduction(t *testing.T) {
	api := handler.NewAPIWithOptions(nil, nil, nil, nil, "", "", "", false, nil, "production", false)
	router := transporthttp.NewRouter(transporthttp.Dependencies{Readiness: func(context.Context) error { return nil }, API: api})
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/auth/dev/login", nil))

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestCapabilitiesExposeDevelopmentFlagWithoutSecrets(t *testing.T) {
	api := handler.NewAPIWithOptions(nil, nil, nil, nil, "", "", "", false, nil, "development", true)
	router := transporthttp.NewRouter(transporthttp.Dependencies{Readiness: func(context.Context) error { return nil }, API: api})
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/auth/capabilities", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"data":{"dev_login":true,"feishu_login":false,"message_providers":[]}}`, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), "secret")
}

func TestDevelopmentLoginCreatesAuthenticatedAdminSession(t *testing.T) {
	db := handlerIntegrationDB(t)
	require.NoError(t, storepostgres.Migrate(context.Background(), db))
	store := storepostgres.New(db)
	tokens := auth.New(db)
	registry := messaging.NewRegistry(store, []messaging.Provider{messagingweb.New()})
	api := handler.NewAPIWithOptions(nil, store, tokens, nil, "", "", "http://localhost:8080", false, registry, "development", true)
	router := transporthttp.NewRouter(transporthttp.Dependencies{Readiness: store.Ready, API: api, Tokens: tokens})
	login := httptest.NewRecorder()

	router.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/auth/dev/login", nil))

	require.Equal(t, http.StatusOK, login.Code)
	cookies := login.Result().Cookies()
	require.Len(t, cookies, 2)
	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	for _, cookie := range cookies {
		meRequest.AddCookie(cookie)
	}
	me := httptest.NewRecorder()
	router.ServeHTTP(me, meRequest)
	require.Equal(t, http.StatusOK, me.Code)
	require.Contains(t, me.Body.String(), "Local Development Administrator")
}

func TestAdminCanSelectConfiguredProvider(t *testing.T) {
	db := handlerIntegrationDB(t)
	require.NoError(t, storepostgres.Migrate(context.Background(), db))
	store := storepostgres.New(db)
	tokens := auth.New(db)
	registry := messaging.NewRegistry(store, []messaging.Provider{
		messagingweb.New(),
		dingtalk.NewProvider(config.DingTalkConfig{ClientID: "client", ClientSecret: "secret", RobotCode: "robot", EventToken: "token", APIBaseURL: "https://example.test"}),
	})
	api := handler.NewAPIWithOptions(nil, store, tokens, nil, "", "", "http://localhost:8080", false, registry, "development", true)
	router := transporthttp.NewRouter(transporthttp.Dependencies{Readiness: store.Ready, API: api, Tokens: tokens})
	login := httptest.NewRecorder()
	router.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/auth/dev/login", nil))
	require.Equal(t, http.StatusOK, login.Code)

	request := httptest.NewRequest(http.MethodPut, "/api/v1/settings/message-provider", strings.NewReader(`{"provider":"dingtalk"}`))
	for _, cookie := range login.Result().Cookies() {
		request.AddCookie(cookie)
		if cookie.Name == "chatops_csrf" {
			request.Header.Set("X-CSRF-Token", cookie.Value)
		}
	}
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"data":{"provider":"dingtalk"}}`, response.Body.String())
}

func handlerIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	rawURL := os.Getenv("CHATOPS_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("CHATOPS_TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	adminDB, adminSQL, err := storepostgres.Open(context.Background(), rawURL, 2, 1, time.Minute)
	require.NoError(t, err)
	schema := "handler_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, adminDB.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error)
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, sqlDB, err := storepostgres.Open(context.Background(), parsed.String(), 4, 2, time.Minute)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		_ = adminDB.Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE`, schema)).Error
		_ = adminSQL.Close()
	})
	return db
}
