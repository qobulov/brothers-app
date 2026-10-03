package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort                      string
	AppEnv                       string
	APIRequestTimeout            time.Duration
	APICacheTTL                  time.Duration
	DBHost                       string
	DBPort                       string
	DBUser                       string
	DBPassword                   string
	DBName                       string
	DatabaseDSN                  string
	RedisURL                     string
	CORSAllowOrigins             string
	CORSAllowCredentials         bool
	TelegramBotToken             string
	TelegramChatID               string
	TelegramBackendErrorThreadID int

	JWTSecret     string
	JWTExpiration int // in seconds
	JWTIssuer     string
	JWTAudience   string

	SMTPHost          string
	SMTPPort          int
	SMTPUsername      string
	SMTPPassword      string
	SMTPFrom          string
	OTPPepper         string
	OTPExpiration     int
	OTPResendCooldown int
	OTPMaxAttempts    int
}

func LoadConfig(env string) *Config {
	envFile := ".env"
	if env != "" {
		envFile = ".env." + env
	}

	if err := godotenv.Load(envFile); err != nil {
		log.Println("No .env file found, using system env", err)
	}

	jwtExp := getEnvAsInt("JWT_EXPIRATION", 3600)

	cfg := &Config{
		AppPort:                      getEnv("PORT", getEnv("APP_PORT", "8000")),
		AppEnv:                       getEnv("APP_ENV", "development"),
		APIRequestTimeout:            getEnvAsDuration("API_REQUEST_TIMEOUT", 10*time.Second),
		APICacheTTL:                  getEnvAsDuration("API_CACHE_TTL", 30*time.Second),
		DBHost:                       getEnv("DB_HOST", "localhost"),
		DBPort:                       getEnv("DB_PORT", "5432"),
		DBUser:                       getEnv("DB_USER", "postgres"),
		DBPassword:                   getEnv("DB_PASSWORD", "brothers"),
		DBName:                       getEnv("DB_NAME", "test"),
		RedisURL:                     getEnv("REDIS_URL", "redis://localhost:6379/0"),
		CORSAllowOrigins:             getEnv("CORS_ALLOW_ORIGINS", "*"),
		CORSAllowCredentials:         getEnvAsBool("CORS_ALLOW_CREDENTIALS", false),
		TelegramBotToken:             getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:               getEnv("TELEGRAM_CHAT_ID", "-1003866068293"),
		TelegramBackendErrorThreadID: getEnvAsInt("TELEGRAM_BACKEND_ERROR_THREAD_ID", 0),
		JWTSecret:                    getEnv("JWT_SECRET", "changeme"),
		JWTExpiration:                jwtExp,
		JWTIssuer:                    getEnv("JWT_ISSUER", "brothers-app"),
		JWTAudience:                  getEnv("JWT_AUDIENCE", "brothers-api"),
		SMTPHost:                     getEnv("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:                     getEnvAsInt("SMTP_PORT", 587),
		SMTPUsername:                 getEnv("SMTP_USERNAME", ""),
		SMTPPassword:                 getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:                     getEnv("SMTP_FROM", ""),
		OTPPepper:                    getEnv("OTP_PEPPER", "development-only-change-me"),
		OTPExpiration:                getEnvAsInt("OTP_EXPIRATION", 120),
		OTPResendCooldown:            getEnvAsInt("OTP_RESEND_COOLDOWN", 60),
		OTPMaxAttempts:               getEnvAsInt("OTP_MAX_ATTEMPTS", 5),
	}

	databaseDSNFallback := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName,
	)
	cfg.DatabaseDSN = getEnv("DATABASE_URL", databaseDSNFallback)

	return cfg
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return fallback
}

func getEnvAsBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseBool(val); err == nil {
			return parsed
		}
	}
	return fallback
}

func getEnvAsDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}
