package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds every runtime knob. Everything is read from the environment,
// which is pre-populated from a .env file when one exists.
type Config struct {
	Addr    string
	BaseURL string

	DatabaseURL    string
	DBMaxOpenConns int
	AutoMigrate    bool

	CookieName   string
	CookieSecure bool
	SessionDays  int

	DefaultSpinSeconds int
	MinSpinSeconds     int
	MaxSpinSeconds     int
	MaxEntries         int
	PlayRetentionDays  int

	LogLevel        slog.Level
	LogFormat       string
	ShutdownTimeout time.Duration
}

// Load populates the environment from .env (if present) and then builds a
// validated Config. Real environment variables always take precedence over
// values in the file.
func Load() (*Config, error) {
	for _, path := range envFileCandidates() {
		if err := LoadDotEnv(path); err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
	}

	cfg := &Config{
		Addr:               envString("ADDR", ":8080"),
		BaseURL:            strings.TrimRight(envString("BASE_URL", ""), "/"),
		DatabaseURL:        envString("DATABASE_URL", ""),
		DBMaxOpenConns:     envInt("DB_MAX_OPEN_CONNS", 25),
		AutoMigrate:        envBool("AUTO_MIGRATE", true),
		CookieName:         envString("COOKIE_NAME", "wheel_sid"),
		CookieSecure:       envBool("COOKIE_SECURE", false),
		SessionDays:        envInt("SESSION_DAYS", 365),
		DefaultSpinSeconds: envInt("DEFAULT_SPIN_SECONDS", 15),
		MinSpinSeconds:     envInt("MIN_SPIN_SECONDS", 3),
		MaxSpinSeconds:     envInt("MAX_SPIN_SECONDS", 300),
		MaxEntries:         envInt("MAX_ENTRIES", 500),
		PlayRetentionDays:  envInt("PLAY_RETENTION_DAYS", 60),
		LogFormat:          strings.ToLower(envString("LOG_FORMAT", "text")),
		ShutdownTimeout:    envDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
	}

	// PORT alone is enough for hosts that only inject a port number.
	if port := os.Getenv("PORT"); port != "" && os.Getenv("ADDR") == "" {
		cfg.Addr = ":" + port
	}
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = databaseURLFromParts()
	}

	level, err := parseLevel(envString("LOG_LEVEL", "info"))
	if err != nil {
		return nil, err
	}
	cfg.LogLevel = level

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set (or set PGHOST/PGUSER/PGDATABASE instead)")
	}
	if _, err := url.Parse(c.DatabaseURL); err != nil {
		return fmt.Errorf("DATABASE_URL is not a valid URL: %w", err)
	}
	if c.Addr == "" {
		return fmt.Errorf("ADDR is empty")
	}
	if c.BaseURL != "" {
		u, err := url.Parse(c.BaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("BASE_URL must be an absolute URL like https://wheel.example.com")
		}
	}
	if c.DBMaxOpenConns < 1 {
		c.DBMaxOpenConns = 1
	}
	if c.SessionDays < 1 {
		c.SessionDays = 1
	}
	if c.MaxEntries < 2 {
		c.MaxEntries = 2
	}
	if c.PlayRetentionDays < 0 {
		c.PlayRetentionDays = 0
	}
	if c.MinSpinSeconds < 1 {
		c.MinSpinSeconds = 1
	}
	if c.MaxSpinSeconds < c.MinSpinSeconds {
		return fmt.Errorf("MAX_SPIN_SECONDS (%d) is below MIN_SPIN_SECONDS (%d)", c.MaxSpinSeconds, c.MinSpinSeconds)
	}
	c.DefaultSpinSeconds = c.ClampSeconds(c.DefaultSpinSeconds)
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = 10 * time.Second
	}
	if c.LogFormat != "json" {
		c.LogFormat = "text"
	}
	return nil
}

// ClampSeconds keeps a requested spin duration inside the configured bounds.
func (c *Config) ClampSeconds(v int) int {
	if v < c.MinSpinSeconds {
		return c.MinSpinSeconds
	}
	if v > c.MaxSpinSeconds {
		return c.MaxSpinSeconds
	}
	return v
}

// envFileCandidates lists the .env locations to try, nearest first. An explicit
// ENV_FILE wins and is the only file consulted.
func envFileCandidates() []string {
	if p := os.Getenv("ENV_FILE"); p != "" {
		return []string{p}
	}
	paths := []string{".env"}
	if exe, err := os.Executable(); err == nil {
		if next := filepath.Join(filepath.Dir(exe), ".env"); next != ".env" {
			paths = append(paths, next)
		}
	}
	return paths
}

func databaseURLFromParts() string {
	host := envString("PGHOST", "")
	user := envString("PGUSER", "")
	name := envString("PGDATABASE", "")
	if host == "" && user == "" && name == "" {
		return ""
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if user == "" {
		user = "postgres"
	}
	if name == "" {
		name = user
	}

	u := &url.URL{
		Scheme: "postgres",
		Host:   host,
		Path:   "/" + name,
	}
	if port := envString("PGPORT", ""); port != "" {
		u.Host = host + ":" + port
	}
	if pass, ok := os.LookupEnv("PGPASSWORD"); ok {
		u.User = url.UserPassword(user, pass)
	} else {
		u.User = url.User(user)
	}
	q := url.Values{}
	q.Set("sslmode", envString("PGSSLMODE", "disable"))
	u.RawQuery = q.Encode()
	return u.String()
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL %q is not one of debug, info, warn, error", s)
	}
}

func envString(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return b
}

func envDuration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return d
}
