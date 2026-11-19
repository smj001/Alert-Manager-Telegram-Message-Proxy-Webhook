package config

import (
	"errors"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	BotToken   string
	APIKey     string
	ServerPort string
	LogLevel   slog.Level
}

var tokenPattern = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]+$`)

func LoadConfig() (Config, error) {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if !tokenPattern.MatchString(botToken) {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN must be a Telegram bot token")
	}
	apiKey := os.Getenv("WEBHOOK_API_KEY")
	if apiKey == "" || apiKey == "your_api_key_here" || strings.IndexFunc(apiKey, func(r rune) bool { return r < '!' || r > '~' }) != -1 {
		return Config{}, errors.New("WEBHOOK_API_KEY must be set to a non-placeholder value with no spaces or non-ASCII characters")
	}
	port, err := strconv.ParseUint(envOr("SERVER_PORT", "8080"), 10, 16)
	if err != nil || port == 0 {
		return Config{}, errors.New("SERVER_PORT must be an integer between 1 and 65535")
	}
	var level slog.Level
	switch strings.ToLower(envOr("LOG_LEVEL", "info")) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return Config{}, errors.New("LOG_LEVEL must be debug, info, warn, or error")
	}
	return Config{BotToken: botToken, APIKey: apiKey, ServerPort: strconv.FormatUint(port, 10), LogLevel: level}, nil
}

func envOr(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
