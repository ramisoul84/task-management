// Package config loads application configuration from the environment.
//
// Nothing is hardcoded: every value comes from an environment variable with
// a sensible default, and secrets have no default at all. In non-production
// environments a .env.dev file is loaded first for convenience.
package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/ramisoul84/task-management/pkg/database"
	"github.com/ramisoul84/task-management/pkg/logger"
	"github.com/ramisoul84/task-management/pkg/redisx"
)

const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvProduction  = "production"

	minProductionSecretLen = 32
)

// Config is the root configuration.
type Config struct {
	App       AppConfig
	Log       logger.Config
	HTTP      HTTPConfig
	DB        database.Config
	Redis     redisx.Config
	Breaker   BreakerConfig
	Auth      AuthConfig
	Cache     CacheConfig
	RateLimit RateLimitConfig
}

type AppConfig struct {
	Env            string
	Name           string
	Version        string
	MigrationsDir  string
	MigrateOnStart bool
}

type HTTPConfig struct {
	Port             int
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
	ShutdownTimeout  time.Duration
	RequestTimeout   time.Duration
	BodyLimitBytes   int
	AllowedOrigins   []string
	AllowCredentials bool
	TrustedProxies   []string
	EnableDocs       bool
	EnableMetrics    bool
}

type AuthConfig struct {
	Secret          string
	Issuer          string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	BcryptCost      int
	CookieDomain    string
	CookieSecure    bool
	CookieSameSite  string
}

type BreakerConfig struct {
	Enabled             bool
	ConsecutiveFailures int
	OpenTimeout         time.Duration
}

type CacheConfig struct {
	TaskListTTL time.Duration
}

type RateLimitConfig struct {
	Enabled      bool
	GlobalMax    int
	GlobalWindow time.Duration
	AuthMax      int
	AuthWindow   time.Duration
}

// Load reads configuration for the given environment. It returns every
// invalid or missing variable in a single joined error.
func Load() (*Config, error) {
	e := &env{}

	appEnv := strings.ToLower(e.str("APP_ENV", EnvDevelopment))
	if appEnv != EnvProduction {
		// Missing file is fine: variables may come from the real environment.
		_ = godotenv.Load(e.str("ENV_FILE", ".env.dev"))
	}

	defaultLogFormat := "json"
	if appEnv == EnvDevelopment {
		defaultLogFormat = "console"
	}

	cfg := &Config{
		App: AppConfig{
			Env:            appEnv,
			Name:           e.str("APP_NAME", "task-management"),
			Version:        e.str("APP_VERSION", "dev"),
			MigrationsDir:  e.str("MIGRATIONS_DIR", "./migrations"),
			MigrateOnStart: e.bool("MIGRATE_ON_START", true),
		},

		Log: logger.Config{
			Level:  e.str("LOG_LEVEL", "info"),
			Format: e.str("LOG_FORMAT", defaultLogFormat),
			Output: e.str("LOG_OUTPUT", "stdout"),
		},

		HTTP: HTTPConfig{
			Port:             e.int("HTTP_PORT", 8080),
			ReadTimeout:      e.duration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:     e.duration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:      e.duration("HTTP_IDLE_TIMEOUT", 120*time.Second),
			ShutdownTimeout:  e.duration("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second),
			RequestTimeout:   e.duration("HTTP_REQUEST_TIMEOUT", 15*time.Second),
			BodyLimitBytes:   e.int("BODY_LIMIT_MB", 1) << 20,
			AllowedOrigins:   e.list("ALLOWED_ORIGINS", "*"),
			AllowCredentials: e.bool("ALLOW_CREDENTIALS", false),
			TrustedProxies:   e.list("TRUSTED_PROXIES", ""),
			EnableDocs:       e.bool("ENABLE_DOCS", true),
			EnableMetrics:    e.bool("ENABLE_METRICS", true),
		},

		DB: database.Config{
			Host:            e.str("DB_HOST", "localhost"),
			Port:            e.str("DB_PORT", "3306"),
			User:            e.required("DB_USER"),
			Password:        e.required("DB_PASSWORD"),
			Name:            e.required("DB_NAME"),
			TLS:             e.str("DB_TLS", "false"),
			MaxOpenConns:    e.int("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    e.int("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: e.duration("DB_CONN_LIFETIME", 5*time.Minute),
			ConnMaxIdleTime: e.duration("DB_CONN_IDLE_TIME", time.Minute),
		},

		Redis: redisx.Config{
			Addr:     e.str("REDIS_ADDR", "localhost:6379"),
			Password: e.str("REDIS_PASSWORD", ""),
			DB:       e.int("REDIS_DB", 0),
			// Redis sits on the request path (cache, rate limit, token
			// checks), so keep timeouts tight; the breaker handles outages.
			MaxRetries:   e.int("REDIS_MAX_RETRIES", 2),
			DialTimeout:  e.duration("REDIS_DIAL_TIMEOUT", 2*time.Second),
			ReadTimeout:  e.duration("REDIS_READ_TIMEOUT", time.Second),
			WriteTimeout: e.duration("REDIS_WRITE_TIMEOUT", time.Second),
			PoolSize:     e.int("REDIS_POOL_SIZE", 20),
			MinIdleConns: e.int("REDIS_MIN_IDLE_CONNS", 5),
		},

		Breaker: BreakerConfig{
			Enabled:             e.bool("REDIS_BREAKER_ENABLED", true),
			ConsecutiveFailures: e.int("REDIS_BREAKER_FAILURES", 5),
			OpenTimeout:         e.duration("REDIS_BREAKER_OPEN_TIMEOUT", 15*time.Second),
		},

		Auth: AuthConfig{
			Secret:          e.required("SECRET"),
			Issuer:          e.str("ISSUER", "task-management"),
			AccessTokenTTL:  e.duration("ACCESS_TOKEN_TTL", 15*time.Minute),
			RefreshTokenTTL: e.duration("REFRESH_TOKEN_TTL", 7*24*time.Hour),
			BcryptCost:      e.int("COST", 12),
			CookieDomain:    e.str("COOKIE_DOMAIN", ""),
			CookieSecure:    e.bool("COOKIE_SECURE", appEnv == EnvProduction),
			CookieSameSite:  e.str("COOKIE_SAME_SITE", "Lax"),
		},

		Cache: CacheConfig{
			TaskListTTL: e.duration("CACHE_TASK_LIST_TTL", 5*time.Minute),
		},

		RateLimit: RateLimitConfig{
			Enabled:      e.bool("RATE_LIMIT_ENABLED", true),
			GlobalMax:    e.int("GLOBAL_RATE_MAX", 100),
			GlobalWindow: e.duration("GLOBAL_RATE_WINDOW", time.Minute),
			AuthMax:      e.int("AUTH_RATE_MAX", 20),
			AuthWindow:   e.duration("AUTH_RATE_WINDOW", time.Minute),
		},
	}

	cfg.Log.Service = cfg.App.Name
	cfg.Log.Version = cfg.App.Version

	cfg.validate(e)

	if len(e.errs) > 0 {
		return nil, fmt.Errorf("invalid configuration:\n%w", errors.Join(e.errs...))
	}

	return cfg, nil
}

func (c *Config) validate(e *env) {
	e.check(slices.Contains([]string{EnvDevelopment, EnvTest, EnvProduction}, c.App.Env),
		"APP_ENV must be one of development, test, production; got %q", c.App.Env)
	e.check(slices.Contains([]string{"json", "console"}, c.Log.Format),
		"LOG_FORMAT must be json or console; got %q", c.Log.Format)
	e.check(c.HTTP.Port > 0 && c.HTTP.Port < 65536, "HTTP_PORT must be between 1 and 65535")
	e.check(c.HTTP.BodyLimitBytes > 0, "BODY_LIMIT_MB must be positive")
	e.check(c.Auth.BcryptCost >= 10 && c.Auth.BcryptCost <= 16, "COST must be between 10 and 16")
	e.check(c.Auth.AccessTokenTTL > 0, "ACCESS_TOKEN_TTL must be positive")
	e.check(c.Auth.RefreshTokenTTL > c.Auth.AccessTokenTTL, "REFRESH_TOKEN_TTL must be longer than ACCESS_TOKEN_TTL")
	e.check(slices.Contains([]string{"Strict", "Lax", "None"}, c.Auth.CookieSameSite),
		"COOKIE_SAME_SITE must be Strict, Lax or None")
	e.check(c.Auth.CookieSameSite != "None" || c.Auth.CookieSecure,
		"COOKIE_SAME_SITE=None requires COOKIE_SECURE=true")
	e.check(c.Cache.TaskListTTL > 0 && c.Cache.TaskListTTL <= time.Hour,
		"CACHE_TASK_LIST_TTL must be between 0 and 1h")
	e.check(c.RateLimit.GlobalMax > 0 && c.RateLimit.AuthMax > 0, "rate limits must be positive")
	e.check(c.Breaker.ConsecutiveFailures > 0 && c.Breaker.ConsecutiveFailures < 1000,
		"REDIS_BREAKER_FAILURES must be between 1 and 999")
	e.check(c.Breaker.OpenTimeout > 0, "REDIS_BREAKER_OPEN_TIMEOUT must be positive")

	// Browsers reject credentialed CORS responses with a wildcard origin.
	e.check(!c.HTTP.AllowCredentials || !slices.Contains(c.HTTP.AllowedOrigins, "*"),
		"ALLOW_CREDENTIALS=true cannot be combined with ALLOWED_ORIGINS=*")

	if c.IsProduction() {
		e.check(len(c.Auth.Secret) >= minProductionSecretLen,
			"SECRET must be at least %d bytes in production", minProductionSecretLen)
	}
}

// WeakSecret reports whether the signing secret is too short for production.
func (c *Config) WeakSecret() bool {
	return len(c.Auth.Secret) < minProductionSecretLen
}

func (c *Config) IsDevelopment() bool { return c.App.Env == EnvDevelopment }
func (c *Config) IsProduction() bool  { return c.App.Env == EnvProduction }
