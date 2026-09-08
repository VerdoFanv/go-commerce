package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
)

// Config is the single source of truth for every tunable in the system.
// All values come from environment variables (12-factor style).
type Config struct {
	AppEnv   string `validate:"required,oneof=development test staging production"`
	AppPort  string `validate:"required,numeric"`
	APIKey   string `validate:"required"`
	LogLevel string `validate:"omitempty,oneof=debug info warn error"`

	DBHost     string `validate:"required"`
	DBPort     string `validate:"required,numeric"`
	DBUser     string `validate:"required"`
	DBPassword string
	DBName     string `validate:"required"`
	DBSSLMode  string `validate:"omitempty,oneof=disable require verify-ca verify-full"`

	RedisAddr     string `validate:"required"`
	RedisPassword string
	RedisDB       int `validate:"min=0"`

	KafkaBrokers       []string `validate:"required,min=1"`
	KafkaTopicProducts string   `validate:"required"`
	KafkaTopicDLQ      string   `validate:"required"`
	KafkaGroupWorker   string   `validate:"required"`
	KafkaGroupNotifier string   `validate:"required"`

	MongoURI string `validate:"required"`
	MongoDB  string `validate:"required"`

	// Optional: when empty, product search is disabled.
	TypesenseAddr   string `validate:"omitempty,url"`
	TypesenseAPIKey string `validate:"omitempty"`

	OTELEnabled  bool
	OTelEndpoint string `validate:"omitempty,hostname_port"`

	JWTSecret       string `validate:"required,min=16"`
	JWTAccessTTL    time.Duration
	JWTRefreshTTL   time.Duration
	BcryptCost      int `validate:"min=4,max=31"`
	ProductCacheTTL time.Duration

	RateLimitMax    int `validate:"min=1"`
	RateLimitWindow time.Duration

	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration

	// MetricsPort is where the worker exposes /metrics (the API exposes it on AppPort).
	MetricsPort string `validate:"required,numeric"`
}

var validate = validator.New()

// Validate reports every invalid field at once — fail fast on boot, not at runtime.
func (c Config) Validate() error {
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	return nil
}

// Load reads the environment and panics when the configuration is invalid.
// Panicking on boot is intentional: a misconfigured process must never serve traffic.
func Load() Config {
	cfg := Config{
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

		KafkaBrokers:       envList("KAFKA_BROKERS", "localhost:9092"),
		KafkaTopicProducts: env("KAFKA_TOPIC_PRODUCTS", "products.events"),
		KafkaTopicDLQ:      env("KAFKA_TOPIC_DLQ", "products.events.dlq"),
		KafkaGroupWorker:   env("KAFKA_GROUP_WORKER", "golang-be-worker"),
		KafkaGroupNotifier: env("KAFKA_GROUP_NOTIFIER", "golang-be-notifier"),

		MongoURI: env("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:  env("MONGO_DB", "golang_be_audit"),

		TypesenseAddr: env("TYPESENSE_ADDR", "http://typesense:8108"),
		TypesenseAPIKey: env("TYPESENSE_API_KEY", "dev-typesense-key"),

		OTELEnabled:  envBool("OTEL_ENABLED", false),
		OTelEndpoint: env("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),

		JWTSecret:       env("JWT_SECRET", "dev-secret-change-me"),
		JWTAccessTTL:    envDuration("JWT_ACCESS_TTL", 15*time.Minute),
		JWTRefreshTTL:   envDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		BcryptCost:      envInt("BCRYPT_COST", 10),
		ProductCacheTTL: envDuration("PRODUCT_CACHE_TTL", 5*time.Minute),

		RateLimitMax:    envInt("RATE_LIMIT_MAX", 100),
		RateLimitWindow: envDuration("RATE_LIMIT_WINDOW", time.Minute),

		RequestTimeout:  envDuration("REQUEST_TIMEOUT", 10*time.Second),
		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		MetricsPort:     env("METRICS_PORT", "2112"),
	}

	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	return cfg
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

func (c Config) IsProduction() bool {
	return c.AppEnv == "production"
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

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
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

func envList(key, fallback string) []string {
	v := os.Getenv(key)
	if v == "" {
		v = fallback
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
