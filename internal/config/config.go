package config

import (
	"fmt"
	"time"
)

const (
	DatabaseURLEnv  = "WORKOUT_SERVICE_DATABASE_URL"
	ListenAddrEnv   = "WORKOUT_SERVICE_LISTEN_ADDR"
	ReadTimeoutEnv  = "WORKOUT_SERVICE_READ_TIMEOUT"
	WriteTimeoutEnv = "WORKOUT_SERVICE_WRITE_TIMEOUT"
)

type Config struct {
	DatabaseURL  string
	ListenAddr   string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func LoadFromEnv(environ map[string]string) (Config, error) {
	cfg := Config{
		ListenAddr:   ":8080",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	if environ != nil {
		cfg.DatabaseURL = environ[DatabaseURLEnv]
		if value := environ[ListenAddrEnv]; value != "" {
			cfg.ListenAddr = value
		}
		if value := environ[ReadTimeoutEnv]; value != "" {
			duration, err := time.ParseDuration(value)
			if err != nil {
				return Config{}, fmt.Errorf("parse %s: %w", ReadTimeoutEnv, err)
			}
			cfg.ReadTimeout = duration
		}
		if value := environ[WriteTimeoutEnv]; value != "" {
			duration, err := time.ParseDuration(value)
			if err != nil {
				return Config{}, fmt.Errorf("parse %s: %w", WriteTimeoutEnv, err)
			}
			cfg.WriteTimeout = duration
		}
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("%s is required", DatabaseURLEnv)
	}

	return cfg, nil
}
