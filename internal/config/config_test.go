package config

import "testing"

func TestLoadFromEnvDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFromEnv(map[string]string{
		DatabaseURLEnv: "postgres://example",
	})
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, ":8080")
	}
	if cfg.ReadTimeout.String() != "5s" {
		t.Fatalf("ReadTimeout = %s, want 5s", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout.String() != "10s" {
		t.Fatalf("WriteTimeout = %s, want 10s", cfg.WriteTimeout)
	}
}

func TestLoadFromEnvRequiresDatabaseURL(t *testing.T) {
	t.Parallel()

	if _, err := LoadFromEnv(map[string]string{}); err == nil {
		t.Fatal("LoadFromEnv() error = nil, want non-nil")
	}
}

func TestLoadFromEnvRejectsBadDurations(t *testing.T) {
	t.Parallel()

	if _, err := LoadFromEnv(map[string]string{
		DatabaseURLEnv: "postgres://example",
		ReadTimeoutEnv: "later",
	}); err == nil {
		t.Fatal("LoadFromEnv() error = nil, want non-nil")
	}
}
