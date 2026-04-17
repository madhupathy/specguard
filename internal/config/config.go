package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Storage  StorageConfig
	GitHub   GitHubConfig
	AI       AIConfig
}

type ServerConfig struct {
	Port            string
	Host            string
	ShutdownTimeout time.Duration
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type StorageConfig struct {
	Type      string // "s3" or "local"
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	Endpoint  string // for S3-compatible storage
	LocalPath string // for local storage
}

type GitHubConfig struct {
	AppID          int64
	PrivateKeyPath string
	WebhookSecret  string
}

type AIConfig struct {
	Enabled     bool
	Provider    string // "openai", "anthropic", etc.
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float64
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:            getEnv("PORT", "8080"),
			Host:            getEnv("HOST", "0.0.0.0"),
			ShutdownTimeout: getDurationEnv("SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "specguard"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "specguard"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Storage: StorageConfig{
			Type:      getEnv("STORAGE_TYPE", "local"),
			Bucket:    getEnv("STORAGE_BUCKET", "specguard-artifacts"),
			Region:    getEnv("STORAGE_REGION", "us-east-1"),
			AccessKey: getEnv("STORAGE_ACCESS_KEY", ""),
			SecretKey: getEnv("STORAGE_SECRET_KEY", ""),
			Endpoint:  getEnv("STORAGE_ENDPOINT", ""),
			LocalPath: getEnv("STORAGE_LOCAL_PATH", "./artifacts"),
		},
		GitHub: GitHubConfig{
			AppID:          getInt64Env("GITHUB_APP_ID", 0),
			PrivateKeyPath: getEnv("GITHUB_PRIVATE_KEY_PATH", ""),
			WebhookSecret:  getEnv("GITHUB_WEBHOOK_SECRET", ""),
		},
		AI: AIConfig{
			Enabled:     getBoolEnv("AI_ENABLED", false),
			Provider:    getEnv("AI_PROVIDER", "openai"),
			APIKey:      getEnv("AI_API_KEY", ""),
			Model:       getEnv("AI_MODEL", "gpt-4"),
			MaxTokens:   getIntEnv("AI_MAX_TOKENS", 2000),
			Temperature: getFloat64Env("AI_TEMPERATURE", 0.7),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getInt64Env(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getFloat64Env(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
