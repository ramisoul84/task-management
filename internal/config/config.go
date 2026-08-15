package config

import "time"

type Config struct {
	App      AppConfig
	Logging  LoggingConfig
	HTTP     HTTPConfig
	DB       MySQLConfig
	Cache    RedisConfig
	Security SecurityConfig
}

type AppConfig struct {
	Env     string
	Name    string
	Version string
}

type LoggingConfig struct {
	Level string
}

type HTTPConfig struct {
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	AllowedOrigins  string
	BodyLimitMB     int
}

type MySQLConfig struct {
	Host         string
	Port         string
	User         string
	Password     string
	DBName       string
	TLS          string
	MaxOpenConns int
	MaxIdleConns int
	ConnLifetime time.Duration
	ConnIdleTime time.Duration
}

type RedisConfig struct {
	Addr         string
	Password     string
	DB           int
	MaxRetries   int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolSize     int
	MinIdleConns int
}

type SecurityConfig struct {
	Secret          string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	CookieDomain    string
	CookieSecure    bool
	CookieSameSite  string
	Issuer          string
	Cost            int
}

func Load(env string) *Config {
	loadDotenv(env)

	cfg := &Config{
		App: AppConfig{
			Env:     getEnv("APP_ENV", "development"),
			Name:    getEnv("APP_NAME", "task-management"),
			Version: getEnv("APP_VERSION", "1.0.0"),
		},

		Logging: LoggingConfig{
			Level: getEnv("LOG_LEVEL", "debug"),
		},

		HTTP: HTTPConfig{
			Port:            getEnvInt("HTTP_PORT", 8000),
			ReadTimeout:     getEnvDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:    getEnvDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:     getEnvDuration("HTTP_IDLE_TIMEOUT", 120*time.Second),
			ShutdownTimeout: getEnvDuration("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second),
			AllowedOrigins:  getEnv("ALLOWED_ORIGINS", "*"),
			BodyLimitMB:     getEnvInt("BODY_LIMIT_MB", 10),
		},

		DB: MySQLConfig{
			Host:         getEnv("DB_HOST", "localhost"),
			Port:         getEnv("DB_PORT", "3306"),
			User:         mustGetEnv("DB_USER"),
			Password:     mustGetEnv("DB_PASSWORD"),
			DBName:       mustGetEnv("DB_NAME"),
			TLS:          getEnv("DB_TLS", "false"),
			MaxOpenConns: getEnvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns: getEnvInt("DB_MAX_IDLE_CONNS", 10),
			ConnLifetime: getEnvDuration("DB_CONN_LIFETIME", 5*time.Minute),
			ConnIdleTime: getEnvDuration("DB_CONN_IDLE_TIME", 1*time.Minute),
		},

		Cache: RedisConfig{
			Addr:         getEnv("REDIS_ADDR", "localhost:6379"),
			Password:     getEnv("REDIS_PASSWORD", ""),
			DB:           getEnvInt("REDIS_DB", 0),
			MaxRetries:   getEnvInt("REDIS_MAX_RETRIES", 3),
			DialTimeout:  getEnvDuration("REDIS_DIAL_TIMEOUT", 5*time.Second),
			ReadTimeout:  getEnvDuration("REDIS_READ_TIMEOUT", 3*time.Second),
			WriteTimeout: getEnvDuration("REDIS_WRITE_TIMEOUT", 3*time.Second),
			PoolSize:     getEnvInt("REDIS_POOL_SIZE", 20),
			MinIdleConns: getEnvInt("REDIS_MIN_IDLE_CONNS", 5),
		},

		Security: SecurityConfig{
			Secret:          mustGetEnv("SECRET"),
			AccessTokenTTL:  getEnvDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvDuration("REFRESH_TOKEN_TTL", 24*time.Hour),
			CookieDomain:    getEnv("COOKIE_DOMAIN", ""),
			CookieSecure:    getEnvBool("COOKIE_SECURE", false),
			CookieSameSite:  getEnv("COOKIE_SAME_SITE", "LAX"),
			Issuer:          getEnv("ISSUER", "task-management"),
			Cost:            getEnvInt("COST", 12),
		},
	}
	return cfg
}

func (c *Config) IsDevelopment() bool {
	return c.App.Env == "development"
}

func (c *Config) IsProduction() bool {
	return c.App.Env == "production"
}
