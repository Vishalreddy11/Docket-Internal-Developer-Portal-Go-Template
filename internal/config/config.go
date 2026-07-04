// Package config loads all runtime configuration from environment variables.
// Every key the app reads lives here so the README can stay accurate.
package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	App      AppConfig
	Postgres PostgresConfig
	NATS     NATSConfig
	Redis    RedisConfig
	S3       S3Config
	OTel     OTelConfig
}

type AppConfig struct {
	Port     string
	APIKey   string
	LogLevel string
}

type PostgresConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DB       string
	SSLMode  string
}

func (p PostgresConfig) DSN() string {
	return "postgres://" + p.User + ":" + p.Password + "@" + p.Host + ":" + p.Port + "/" + p.DB + "?sslmode=" + p.SSLMode
}

type NATSConfig struct {
	URL           string
	Stream        string
	SubjectPrefix string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// S3Config is the config for any S3-compatible object store.
// In this template it points at SeaweedFS; production forks typically point
// at AWS S3 or a Ceph RGW endpoint. The Go code uses the MinIO Go SDK, which
// is a generic S3 client.
type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type OTelConfig struct {
	Endpoint    string
	ServiceName string
	Sampler     string
}

func Load() Config {
	return Config{
		App: AppConfig{
			Port:     env("DOCKET_PORT", "8080"),
			APIKey:   env("DOCKET_API_KEY", ""),
			LogLevel: env("LOG_LEVEL", "info"),
		},
		Postgres: PostgresConfig{
			Host:     env("POSTGRES_HOST", "localhost"),
			Port:     env("POSTGRES_PORT", "5432"),
			User:     env("POSTGRES_USER", "docket"),
			Password: env("POSTGRES_PASSWORD", "docket"),
			DB:       env("POSTGRES_DB", "docket"),
			SSLMode:  env("POSTGRES_SSLMODE", "disable"),
		},
		NATS: NATSConfig{
			URL:           env("NATS_URL", "nats://localhost:4222"),
			Stream:        env("NATS_STREAM", "DOCKET_EVENTS"),
			SubjectPrefix: env("NATS_SUBJECT_PREFIX", "docket.files"),
		},
		Redis: RedisConfig{
			Addr:     env("REDIS_ADDR", "localhost:6379"),
			Password: env("REDIS_PASSWORD", ""),
			DB:       atoi(env("REDIS_DB", "0")),
		},
		S3: S3Config{
			Endpoint:  env("S3_ENDPOINT", "localhost:9000"),
			AccessKey: env("S3_ACCESS_KEY", "docketaccesskey"),
			SecretKey: env("S3_SECRET_KEY", "docketsecretkey"),
			Bucket:    env("S3_BUCKET", "docket"),
			UseSSL:    env("S3_USE_SSL", "false") == "true",
		},
		OTel: OTelConfig{
			Endpoint:    env("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
			ServiceName: env("OTEL_SERVICE_NAME", "docket"),
			Sampler:     env("OTEL_TRACES_SAMPLER", "always_on"),
		},
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
