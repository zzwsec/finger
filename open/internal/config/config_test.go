package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	setCommonEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Install.Domain != "/p8" || cfg.Install.GroupID != 1 {
		t.Fatalf("Load() install = %+v", cfg.Install)
	}
	if cfg.Install.GameDatabase.Name != "cbt4_game_" {
		t.Fatalf("Load() GameDatabase.Name = %q, want %q", cfg.Install.GameDatabase.Name, "cbt4_game_")
	}
}

func TestLoadCustomGameDbPrefix(t *testing.T) {
	setCommonEnvironment(t)
	t.Setenv("OPEN_GAME_DATABASE_NAME_PREFIX", "custom_game_")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Install.GameDatabase.Name != "custom_game_" {
		t.Fatalf("Load() GameDatabase.Name = %q, want %q", cfg.Install.GameDatabase.Name, "custom_game_")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("OPEN_CDN_URL", "http://cdn.example/open")
	t.Setenv("OPEN_LOGIN_HOST", "10.0.0.3")
	t.Setenv("OPEN_LOG_DATABASE_HOST", "db.example")
	t.Setenv("OPEN_LOG_DATABASE_PASSWORD", "secret")
	t.Setenv("OPEN_PAY_NOTIFY_URL", "http://api.example/callback")
	t.Setenv("OPEN_ZK_ENDPOINTS", "10.0.0.4:2881")
	t.Setenv("OPEN_GAME_DATABASE_HOST", "game-db.example")
	t.Setenv("OPEN_GAME_DATABASE_PASSWORD", "secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RegisterThreshold != 2000 {
		t.Fatalf("RegisterThreshold = %d, want 2000", cfg.RegisterThreshold)
	}
	if cfg.RechargeThreshold != 100 {
		t.Fatalf("RechargeThreshold = %d, want 100", cfg.RechargeThreshold)
	}
	if cfg.MoneyThreshold != 6 {
		t.Fatalf("MoneyThreshold = %d, want 6", cfg.MoneyThreshold)
	}
	if cfg.PollInterval != 10*time.Second {
		t.Fatalf("PollInterval = %v, want 10s", cfg.PollInterval)
	}
	if cfg.LimitDelay != 300*time.Second {
		t.Fatalf("LimitDelay = %v, want 300s", cfg.LimitDelay)
	}
	if cfg.LogDatabase.Port != 3306 {
		t.Fatalf("LogDatabase.Port = %d, want 3306", cfg.LogDatabase.Port)
	}
	if cfg.LogDatabase.User != "root" {
		t.Fatalf("LogDatabase.User = %q, want root", cfg.LogDatabase.User)
	}
	if cfg.LogDatabase.Name != "cbt4_log" {
		t.Fatalf("LogDatabase.Name = %q, want cbt4_log", cfg.LogDatabase.Name)
	}
	if cfg.Install.Domain != "/p8" {
		t.Fatalf("Domain = %q, want /p8", cfg.Install.Domain)
	}
	if cfg.Install.GameDatabase.Port != 3306 {
		t.Fatalf("GameDatabase.Port = %d, want 3306", cfg.Install.GameDatabase.Port)
	}
	if cfg.Install.GameDatabase.User != "root" {
		t.Fatalf("GameDatabase.User = %q, want root", cfg.Install.GameDatabase.User)
	}
	if cfg.Install.Threads != 8 {
		t.Fatalf("Threads = %d, want 8", cfg.Install.Threads)
	}
	if cfg.Install.GameIndexCount != 2 {
		t.Fatalf("GameIndexCount = %d, want 2", cfg.Install.GameIndexCount)
	}
	if cfg.Install.GroupID != 1 {
		t.Fatalf("GroupID = %d, want 1", cfg.Install.GroupID)
	}
	if cfg.Install.BasePort != 3340 {
		t.Fatalf("BasePort = %d, want 3340", cfg.Install.BasePort)
	}
	if cfg.Install.GameDatabase.Name != "cbt4_game_" {
		t.Fatalf("GameDatabase.Name = %q, want cbt4_game_", cfg.Install.GameDatabase.Name)
	}
}

func TestLoadRequiresInstallSettings(t *testing.T) {
	setCommonEnvironment(t)
	t.Setenv("OPEN_PAY_NOTIFY_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want missing install settings error")
	}
}

func setCommonEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("OPEN_CDN_URL", "http://cdn.example/open")
	t.Setenv("OPEN_LOGIN_HOST", "10.0.0.3")
	t.Setenv("OPEN_LOG_DATABASE_HOST", "db.example")
	t.Setenv("OPEN_LOG_DATABASE_PORT", "3306")
	t.Setenv("OPEN_LOG_DATABASE_USER", "open")
	t.Setenv("OPEN_LOG_DATABASE_PASSWORD", "secret")
	t.Setenv("OPEN_LOG_DATABASE_NAME", "logs")
	t.Setenv("OPEN_REGISTER_THRESHOLD", "100")
	t.Setenv("OPEN_RECHARGE_THRESHOLD", "10")
	t.Setenv("OPEN_MONEY_THRESHOLD", "6")
	t.Setenv("OPEN_DOMAIN", "/p8")
	t.Setenv("OPEN_GAME_THREADS", "8")
	t.Setenv("OPEN_PAY_NOTIFY_URL", "http://api.example/callback")
	t.Setenv("OPEN_ZK_ENDPOINTS", "10.0.0.4:2881")
	t.Setenv("OPEN_GAME_DATABASE_HOST", "game-db.example")
	t.Setenv("OPEN_GAME_DATABASE_PORT", "3306")
	t.Setenv("OPEN_GAME_DATABASE_USER", "game")
	t.Setenv("OPEN_GAME_DATABASE_PASSWORD", "secret")
	t.Setenv("OPEN_GAME_INDEX_COUNT", "2")
	t.Setenv("OPEN_GAME_GROUP_ID", "1")
}
