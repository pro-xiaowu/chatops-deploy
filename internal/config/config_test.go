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
	setConfigFile(t, "database:\n  url: postgres://localhost/chatops\n")

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
	setConfigFile(t, "database:\n  url: postgres://file/chatops\n")
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
	setConfigFile(t, "mode: all\n")

	_, err := config.Load()

	require.EqualError(t, err, "database.url is required")
}

func TestLoadRejectsInvalidMode(t *testing.T) {
	setConfigFile(t, "mode: invalid\ndatabase:\n  url: postgres://localhost/chatops\n")

	_, err := config.Load()

	require.EqualError(t, err, "mode must be one of all, api, worker")
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
	} {
		t.Setenv(key, "")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	t.Setenv("CHATOPS_CONFIG", path)
}
