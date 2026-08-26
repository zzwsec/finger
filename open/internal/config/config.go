package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Database struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
}

type Endpoint struct {
	Host string
	Port int
}

type Install struct {
	Domain         string
	Threads        int
	PayNotifyURL   string
	Discoveries    []Endpoint
	GameDatabase   Database
	GameIndexCount int
	BasePort       int
	GroupID        int
}

type Config struct {
	CDNURL            string
	LoginHost         string
	LogDatabase       Database
	RegisterThreshold int
	RechargeThreshold int
	MoneyThreshold    int
	PollInterval      time.Duration
	LimitDelay        time.Duration
	GamesFile         string
	StateFile         string
	AutomationDir     string
	Install           Install
}

func Load() (Config, error) {
	cfg := Config{
		CDNURL:        os.Getenv("OPEN_CDN_URL"),
		LoginHost:     os.Getenv("OPEN_LOGIN_HOST"),
		GamesFile:     valueOrDefault("OPEN_GAMES_FILE", "/open/config/games.txt"),
		StateFile:     valueOrDefault("OPEN_STATE_FILE", "/open/state/current_game"),
		AutomationDir: valueOrDefault("OPEN_AUTOMATION_DIR", "/open/automation"),
	}
	var err error
	if cfg.LogDatabase, err = loadDatabase("OPEN_LOG_DATABASE", "cbt4_log", true); err != nil {
		return Config{}, err
	}
	if cfg.RegisterThreshold, err = intOrDefault("OPEN_REGISTER_THRESHOLD", 2000); err != nil {
		return Config{}, err
	}
	if cfg.RechargeThreshold, err = intOrDefault("OPEN_RECHARGE_THRESHOLD", 100); err != nil {
		return Config{}, err
	}
	if cfg.MoneyThreshold, err = intOrDefault("OPEN_MONEY_THRESHOLD", 6); err != nil {
		return Config{}, err
	}
	if cfg.PollInterval, err = durationOrDefault("OPEN_POLL_INTERVAL", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.LimitDelay, err = durationOrDefault("OPEN_LIMIT_DELAY", 300*time.Second); err != nil {
		return Config{}, err
	}

	if cfg.Install, err = loadInstall(); err != nil {
		return Config{}, err
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if net.ParseIP(c.LoginHost) == nil {
		return errors.New("OPEN_LOGIN_HOST must be a valid IP address")
	}
	if err := validHTTPURL("OPEN_CDN_URL", c.CDNURL); err != nil {
		return err
	}
	if c.PollInterval <= 0 {
		return errors.New("OPEN_POLL_INTERVAL must be greater than zero")
	}
	if c.LimitDelay < 0 {
		return errors.New("OPEN_LIMIT_DELAY cannot be negative")
	}
	return nil
}

func loadInstall() (Install, error) {
	install := Install{
		Domain:       valueOrDefault("OPEN_DOMAIN", "/p8"),
		PayNotifyURL: os.Getenv("OPEN_PAY_NOTIFY_URL"),
	}
	if err := validHTTPURL("OPEN_PAY_NOTIFY_URL", install.PayNotifyURL); err != nil {
		return Install{}, err
	}

	var err error
	if install.GameDatabase, err = loadDatabase("OPEN_GAME_DATABASE", "", false); err != nil {
		return Install{}, err
	}
	install.GameDatabase.Name = valueOrDefault("OPEN_GAME_DATABASE_NAME_PREFIX", valueOrDefault("OPEN_GAME_DATABASE_PREFIX", valueOrDefault("OPEN_GAME_DATABASE_NAME", "cbt4_game_")))

	if install.Threads, err = intOrDefault("OPEN_GAME_THREADS", 8); err != nil {
		return Install{}, err
	}
	if install.GameIndexCount, err = intOrDefault("OPEN_GAME_INDEX_COUNT", 2); err != nil {
		return Install{}, err
	}
	if install.GroupID, err = intOrDefault("OPEN_GAME_GROUP_ID", 1); err != nil {
		return Install{}, err
	}
	if install.BasePort, err = intOrDefault("OPEN_GAME_BASE_PORT", 3340); err != nil {
		return Install{}, err
	}
	if install.Discoveries, err = endpoints("OPEN_ZK_ENDPOINTS"); err != nil {
		return Install{}, err
	}
	return install, nil
}

func loadDatabase(prefix, defaultName string, requireName bool) (Database, error) {
	db := Database{
		Host:     os.Getenv(prefix + "_HOST"),
		User:     valueOrDefault(prefix+"_USER", "root"),
		Password: os.Getenv(prefix + "_PASSWORD"),
		Name:     valueOrDefault(prefix+"_NAME", defaultName),
	}
	port, err := intOrDefault(prefix+"_PORT", 3306)
	if err != nil {
		return Database{}, err
	}
	db.Port = port

	if db.Host == "" || db.User == "" || db.Password == "" || (requireName && db.Name == "") {
		return Database{}, fmt.Errorf("%s connection settings are incomplete", prefix)
	}
	return db, nil
}

func endpoints(name string) ([]Endpoint, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, fmt.Errorf("%s is required", name)
	}

	parts := strings.Split(raw, ",")
	result := make([]Endpoint, 0, len(parts))
	for _, part := range parts {
		host, portText, err := net.SplitHostPort(strings.TrimSpace(part))
		if err != nil || host == "" {
			return nil, fmt.Errorf("%s contains invalid endpoint %q", name, part)
		}
		port, err := strconv.Atoi(portText)
		if err != nil || port <= 0 || port > 65535 {
			return nil, fmt.Errorf("%s contains invalid port in %q", name, part)
		}
		result = append(result, Endpoint{Host: host, Port: port})
	}
	return result, nil
}

func intOrDefault(name string, fallback int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func durationOrDefault(name string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as 30s or 1m: %w", name, err)
	}
	return value, nil
}

func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func validHTTPURL(name, value string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be a valid http or https URL", name)
	}
	return nil
}
