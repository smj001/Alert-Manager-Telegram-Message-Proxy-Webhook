package config

import (
	"log"
	"os"
)

type Config struct {
	BotToken   string
	APIKey     string
	ServerPort string
}

func LoadConfig() *Config {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	apiKey := os.Getenv("WEBHOOK_API_KEY")
	if apiKey == "" {
		log.Fatal("WEBHOOK_API_KEY environment variable is required")
	}

	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		serverPort = "8080"
	}

	return &Config{
		BotToken:   botToken,
		APIKey:     apiKey,
		ServerPort: serverPort,
	}
}
