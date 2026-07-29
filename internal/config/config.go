package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv   string
	AppPort  string
	APIKey   string
	LogLevel string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	RabbitURL      string
	RabbitExchange string
	RabbitQueue    string

	JWTSecret          string
	JWTAccessTTL       time.Duration
	JWTRefreshTTL      time.Duration
	BcryptCost         int
	ProductCacheTTL    time.Duration
}

func Load() Config {
	return Config{
		AppEnv:   env("APP_ENV", "development"),
		AppPort:  env("APP_PORT", "8080"),
		APIKey:   env("API_KEY", "dev-api-key"),
		LogLevel: env("LOG_LEVEL", "info"),

		DBHost:     env("DB_HOST", "localhost"),
		DBPort:     env("DB_PORT", "5432"),
		DBUser:     env("DB_USER", "postgres"),
		DBPassword: env("DB_PASSWORD", "postgres"),
		DBName:     env("DB_NAME", "golang_be"),
		DBSSLMode:  env("DB_SSLMODE", "disable"),

		RedisAddr:     env("REDIS_ADDR", "localhost:6379"),
		RedisPassword: env("REDIS_PASSWORD", ""),
		RedisDB:       envInt("REDIS_DB", 0),

		RabbitURL:      env("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		RabbitExchange: env("RABBITMQ_EXCHANGE", "golang_be.events"),
		RabbitQueue:    env("RABBITMQ_QUEUE", "golang_be.product.created"),

		JWTSecret:       env("JWT_SECRET", "dev-secret-change-me"),
		JWTAccessTTL:    envDuration("JWT_ACCESS_TTL", 15*time.Minute),
		JWTRefreshTTL:   envDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		BcryptCost:      envInt("BCRYPT_COST", 10),
		ProductCacheTTL: envDuration("PRODUCT_CACHE_TTL", 5*time.Minute),
	}
}

func (c Config) PostgresDSN() string {
	return "host=" + c.DBHost +
		" user=" + c.DBUser +
		" password=" + c.DBPassword +
		" dbname=" + c.DBName +
		" port=" + c.DBPort +
		" sslmode=" + c.DBSSLMode +
		" TimeZone=Asia/Jakarta"
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
