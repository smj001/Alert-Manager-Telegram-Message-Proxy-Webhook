package config

import (
	"os"
	"strconv"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/pkg/logger"
)

type Config struct {
	BotToken   string
	APIKey     string
	ServerPort string
	LogLevel   string
}

func LoadConfig() *Config {
	log := logger.New()

	botToken := getEnvOrFatal("TELEGRAM_BOT_TOKEN", log)
	apiKey := getEnvOrFatal("WEBHOOK_API_KEY", log)
	serverPort := getEnvWithDefault("SERVER_PORT", "8080")
	logLevel := getEnvWithDefault("LOG_LEVEL", "info")

	// Validate port number
	if _, err := strconv.Atoi(serverPort); err != nil {
		log.Fatal("SERVER_PORT must be a valid number")
	}

	return &Config{
		BotToken:   botToken,
		APIKey:     apiKey,
		ServerPort: serverPort,
		LogLevel:   logLevel,
	}
}

func getEnvOrFatal(key string, log *logger.Logger) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatal("Environment variable %s is required", key)
	}
	return value
}

func getEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
