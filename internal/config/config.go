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
	Mode     string         `mapstructure:"mode"`
	HTTP     HTTPConfig     `mapstructure:"http"`
	Database DatabaseConfig `mapstructure:"database"`
	Log      LogConfig      `mapstructure:"log"`
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

func Load() (Config, error) {
	v := viper.New()
	v.SetDefault("mode", "all")
	v.SetDefault("http.addr", ":8080")
	v.SetDefault("http.shutdown_timeout", 15*time.Second)
	v.SetDefault("database.max_open_conns", 20)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", 30*time.Minute)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.development", false)

	v.SetEnvPrefix("CHATOPS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	for _, key := range []string{
		"mode",
		"http.addr",
		"http.shutdown_timeout",
		"database.url",
		"database.max_open_conns",
		"database.max_idle_conns",
		"database.conn_max_lifetime",
		"log.level",
		"log.development",
	} {
		if err := v.BindEnv(key); err != nil {
			return Config{}, fmt.Errorf("bind environment variable %s: %w", key, err)
		}
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
	switch cfg.Mode {
	case "all", "api", "worker":
	default:
		return Config{}, errors.New("mode must be one of all, api, worker")
	}

	return cfg, nil
}
