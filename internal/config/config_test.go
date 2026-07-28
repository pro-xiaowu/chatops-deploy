package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"chatops-deploy/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoadUsesDefaults(t *testing.T) {
	setConfigFile(t, validConfig("postgres://localhost/chatops"))

	cfg, err := config.Load()

	require.NoError(t, err)
	require.Equal(t, "all", cfg.Mode)
	require.Equal(t, ":8080", cfg.HTTP.Addr)
	require.Equal(t, 15*time.Second, cfg.HTTP.ShutdownTimeout)
	require.Equal(t, "postgres://localhost/chatops", cfg.Database.URL)
	require.Equal(t, 20, cfg.Database.MaxOpenConns)
	require.Equal(t, 5, cfg.Database.MaxIdleConns)
	require.Equal(t, 30*time.Minute, cfg.Database.ConnMaxLifetime)
	require.Equal(t, "info", cfg.Log.Level)
	require.False(t, cfg.Log.Development)
}

func TestLoadOverridesEveryFieldFromEnvironment(t *testing.T) {
	setConfigFile(t, validConfig("postgres://file/chatops"))
	t.Setenv("CHATOPS_MODE", "worker")
	t.Setenv("CHATOPS_HTTP_ADDR", ":9090")
	t.Setenv("CHATOPS_HTTP_SHUTDOWN_TIMEOUT", "25s")
	t.Setenv("CHATOPS_DATABASE_URL", "postgres://env/chatops")
	t.Setenv("CHATOPS_DATABASE_MAX_OPEN_CONNS", "40")
	t.Setenv("CHATOPS_DATABASE_MAX_IDLE_CONNS", "10")
	t.Setenv("CHATOPS_DATABASE_CONN_MAX_LIFETIME", "45m")
	t.Setenv("CHATOPS_LOG_LEVEL", "debug")
	t.Setenv("CHATOPS_LOG_DEVELOPMENT", "true")

	cfg, err := config.Load()

	require.NoError(t, err)
	require.Equal(t, "worker", cfg.Mode)
	require.Equal(t, ":9090", cfg.HTTP.Addr)
	require.Equal(t, 25*time.Second, cfg.HTTP.ShutdownTimeout)
	require.Equal(t, "postgres://env/chatops", cfg.Database.URL)
	require.Equal(t, 40, cfg.Database.MaxOpenConns)
	require.Equal(t, 10, cfg.Database.MaxIdleConns)
	require.Equal(t, 45*time.Minute, cfg.Database.ConnMaxLifetime)
	require.Equal(t, "debug", cfg.Log.Level)
	require.True(t, cfg.Log.Development)
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	setConfigFile(t, "mode: all\nsecurity:\n  kubeconfig_master_key: test-master-key\n")

	_, err := config.Load()

	require.EqualError(t, err, "database.url is required")
}

func TestLoadRejectsInvalidMode(t *testing.T) {
	setConfigFile(t, "mode: invalid\ndatabase:\n  url: postgres://localhost/chatops\nsecurity:\n  kubeconfig_master_key: test-master-key\n")

	_, err := config.Load()

	require.EqualError(t, err, "mode must be one of all, api, worker")
}

func TestLoadDefaultsToProductionAndWebProvider(t *testing.T) {
	setConfigFile(t, validConfig("postgres://localhost/chatops"))

	cfg, err := config.Load()

	require.NoError(t, err)
	require.Equal(t, "production", cfg.RuntimeEnvironment)
	require.False(t, cfg.DevAuthEnabled)
	require.Equal(t, "web", cfg.MessageProvider)
}

func TestLoadOverridesRuntimeAndProviderFromEnvironment(t *testing.T) {
	setConfigFile(t, validConfig("postgres://localhost/chatops"))
	t.Setenv("CHATOPS_RUNTIME_ENV", "development")
	t.Setenv("CHATOPS_DEV_AUTH_ENABLED", "true")
	t.Setenv("CHATOPS_MESSAGE_PROVIDER", "wecom")

	cfg, err := config.Load()

	require.NoError(t, err)
	require.Equal(t, "development", cfg.RuntimeEnvironment)
	require.True(t, cfg.DevAuthEnabled)
	require.Equal(t, "wecom", cfg.MessageProvider)
}

func TestLoadRejectsDevelopmentAuthInProduction(t *testing.T) {
	setConfigFile(t, validConfig("postgres://localhost/chatops"))
	t.Setenv("CHATOPS_RUNTIME_ENV", "production")
	t.Setenv("CHATOPS_DEV_AUTH_ENABLED", "true")

	_, err := config.Load()

	require.EqualError(t, err, "development auth requires runtime environment development")
}

func TestLoadRejectsUnknownRuntimeEnvironment(t *testing.T) {
	setConfigFile(t, validConfig("postgres://localhost/chatops"))
	t.Setenv("CHATOPS_RUNTIME_ENV", "staging")

	_, err := config.Load()

	require.EqualError(t, err, "runtime environment must be one of production, development")
}

func TestLoadRejectsUnknownMessageProvider(t *testing.T) {
	setConfigFile(t, validConfig("postgres://localhost/chatops"))
	t.Setenv("CHATOPS_MESSAGE_PROVIDER", "telegram")

	_, err := config.Load()

	require.EqualError(t, err, "message provider must be one of web, feishu, wecom, dingtalk")
}

func TestLoadConfiguresWeComAndDingTalkFromEnvironment(t *testing.T) {
	setConfigFile(t, validConfig("postgres://localhost/chatops"))
	t.Setenv("CHATOPS_WECOM_CORP_ID", "corp")
	t.Setenv("CHATOPS_WECOM_AGENT_ID", "1000001")
	t.Setenv("CHATOPS_WECOM_SECRET", "wecom-secret")
	t.Setenv("CHATOPS_WECOM_TOKEN", "wecom-token")
	t.Setenv("CHATOPS_WECOM_ENCODING_AES_KEY", "aes-key")
	t.Setenv("CHATOPS_WECOM_API_BASE_URL", "https://wecom.example")
	t.Setenv("CHATOPS_DINGTALK_CLIENT_ID", "client")
	t.Setenv("CHATOPS_DINGTALK_CLIENT_SECRET", "dingtalk-secret")
	t.Setenv("CHATOPS_DINGTALK_ROBOT_CODE", "robot")
	t.Setenv("CHATOPS_DINGTALK_EVENT_TOKEN", "event-token")
	t.Setenv("CHATOPS_DINGTALK_EVENT_AES_KEY", "event-aes-key")
	t.Setenv("CHATOPS_DINGTALK_API_BASE_URL", "https://dingtalk.example")

	cfg, err := config.Load()

	require.NoError(t, err)
	require.Equal(t, "corp", cfg.WeCom.CorpID)
	require.Equal(t, "1000001", cfg.WeCom.AgentID)
	require.Equal(t, "wecom-secret", cfg.WeCom.Secret)
	require.Equal(t, "aes-key", cfg.WeCom.EncodingAESKey)
	require.Equal(t, "client", cfg.DingTalk.ClientID)
	require.Equal(t, "robot", cfg.DingTalk.RobotCode)
	require.Equal(t, "event-aes-key", cfg.DingTalk.EventAESKey)
}

func validConfig(databaseURL string) string {
	return "database:\n  url: " + databaseURL + "\nsecurity:\n  kubeconfig_master_key: test-master-key\n"
}

func setConfigFile(t *testing.T, contents string) {
	t.Helper()
	for _, key := range []string{
		"CHATOPS_MODE",
		"CHATOPS_HTTP_ADDR",
		"CHATOPS_HTTP_SHUTDOWN_TIMEOUT",
		"CHATOPS_DATABASE_URL",
		"CHATOPS_DATABASE_MAX_OPEN_CONNS",
		"CHATOPS_DATABASE_MAX_IDLE_CONNS",
		"CHATOPS_DATABASE_CONN_MAX_LIFETIME",
		"CHATOPS_LOG_LEVEL",
		"CHATOPS_LOG_DEVELOPMENT",
		"CHATOPS_RUNTIME_ENV",
		"CHATOPS_DEV_AUTH_ENABLED",
		"CHATOPS_MESSAGE_PROVIDER",
		"CHATOPS_SECURITY_KUBECONFIG_MASTER_KEY",
		"CHATOPS_WECOM_CORP_ID", "CHATOPS_WECOM_AGENT_ID", "CHATOPS_WECOM_SECRET", "CHATOPS_WECOM_TOKEN", "CHATOPS_WECOM_ENCODING_AES_KEY", "CHATOPS_WECOM_API_BASE_URL",
		"CHATOPS_DINGTALK_CLIENT_ID", "CHATOPS_DINGTALK_CLIENT_SECRET", "CHATOPS_DINGTALK_ROBOT_CODE", "CHATOPS_DINGTALK_EVENT_TOKEN", "CHATOPS_DINGTALK_EVENT_AES_KEY", "CHATOPS_DINGTALK_API_BASE_URL",
	} {
		t.Setenv(key, "")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	t.Setenv("CHATOPS_CONFIG", path)
}
