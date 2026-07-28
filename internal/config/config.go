package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Mode               string         `mapstructure:"mode"`
	RuntimeEnvironment string         `mapstructure:"runtime_environment"`
	DevAuthEnabled     bool           `mapstructure:"dev_auth_enabled"`
	MessageProvider    string         `mapstructure:"message_provider"`
	HTTP               HTTPConfig     `mapstructure:"http"`
	Database           DatabaseConfig `mapstructure:"database"`
	Log                LogConfig      `mapstructure:"log"`
	Security           SecurityConfig `mapstructure:"security"`
	Feishu             FeishuConfig   `mapstructure:"feishu"`
	Worker             WorkerConfig   `mapstructure:"worker"`
}

type HTTPConfig struct {
	Addr            string        `mapstructure:"addr"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type DatabaseConfig struct {
	URL             string        `mapstructure:"url"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

type LogConfig struct {
	Level       string `mapstructure:"level"`
	Development bool   `mapstructure:"development"`
}

type SecurityConfig struct {
	KubeconfigMasterKey string `mapstructure:"kubeconfig_master_key"`
	BootstrapAdminToken string `mapstructure:"bootstrap_admin_token"`
	PublicBaseURL       string `mapstructure:"public_base_url"`
	CookieSecure        bool   `mapstructure:"cookie_secure"`
}
type FeishuConfig struct {
	AppID             string `mapstructure:"app_id"`
	AppSecret         string `mapstructure:"app_secret"`
	VerificationToken string `mapstructure:"verification_token"`
	EncryptKey        string `mapstructure:"encrypt_key"`
	APIBaseURL        string `mapstructure:"api_base_url"`
}
type WorkerConfig struct {
	ID           string        `mapstructure:"id"`
	PollInterval time.Duration `mapstructure:"poll_interval"`
	LeaseTTL     time.Duration `mapstructure:"lease_ttl"`
	MaxAttempts  int           `mapstructure:"max_attempts"`
}

func Load() (Config, error) {
	v := viper.New()
	v.SetDefault("mode", "all")
	v.SetDefault("runtime_environment", "production")
	v.SetDefault("dev_auth_enabled", false)
	v.SetDefault("message_provider", "web")
	v.SetDefault("http.addr", ":8080")
	v.SetDefault("http.shutdown_timeout", 15*time.Second)
	v.SetDefault("database.max_open_conns", 20)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", 30*time.Minute)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.development", false)
	v.SetDefault("security.cookie_secure", true)
	v.SetDefault("feishu.api_base_url", "https://open.feishu.cn")
	v.SetDefault("worker.id", "chatops-worker")
	v.SetDefault("worker.poll_interval", time.Second)
	v.SetDefault("worker.lease_ttl", 30*time.Second)
	v.SetDefault("worker.max_attempts", 3)

	v.SetEnvPrefix("CHATOPS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	for _, key := range []string{
		"mode",
		"dev_auth_enabled", "message_provider",
		"http.addr",
		"http.shutdown_timeout",
		"database.url",
		"database.max_open_conns",
		"database.max_idle_conns",
		"database.conn_max_lifetime",
		"log.level",
		"log.development",
		"security.kubeconfig_master_key", "security.bootstrap_admin_token", "security.public_base_url", "security.cookie_secure",
		"feishu.app_id", "feishu.app_secret", "feishu.verification_token", "feishu.encrypt_key", "feishu.api_base_url",
		"worker.id", "worker.poll_interval", "worker.lease_ttl", "worker.max_attempts",
	} {
		if err := v.BindEnv(key); err != nil {
			return Config{}, fmt.Errorf("bind environment variable %s: %w", key, err)
		}
	}
	if err := v.BindEnv("runtime_environment", "CHATOPS_RUNTIME_ENV"); err != nil {
		return Config{}, fmt.Errorf("bind environment variable CHATOPS_RUNTIME_ENV: %w", err)
	}

	if path := strings.TrimSpace(os.Getenv("CHATOPS_CONFIG")); path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		if err := v.ReadInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if !errors.As(err, &notFound) {
				return Config{}, fmt.Errorf("read config: %w", err)
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	if strings.TrimSpace(cfg.Database.URL) == "" {
		return Config{}, errors.New("database.url is required")
	}
	if strings.TrimSpace(cfg.Security.KubeconfigMasterKey) == "" {
		return Config{}, errors.New("security.kubeconfig_master_key is required")
	}
	switch cfg.Mode {
	case "all", "api", "worker":
	default:
		return Config{}, errors.New("mode must be one of all, api, worker")
	}
	switch cfg.RuntimeEnvironment {
	case "production", "development":
	default:
		return Config{}, errors.New("runtime environment must be one of production, development")
	}
	if cfg.DevAuthEnabled && cfg.RuntimeEnvironment != "development" {
		return Config{}, errors.New("development auth requires runtime environment development")
	}
	switch cfg.MessageProvider {
	case "web", "feishu", "wecom", "dingtalk":
	default:
		return Config{}, errors.New("message provider must be one of web, feishu, wecom, dingtalk")
	}

	return cfg, nil
}
