package main

import (
	"log"
	"net/http"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/config"
	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/handlers"
	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/services"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Initialize Telegram service
	telegramService, err := services.NewTelegramService(cfg.BotToken)
	if err != nil {
		log.Fatalf("Failed to initialize Telegram service: %v", err)
	}

	// Initialize webhook handler
	webhookHandler := handlers.NewWebhookHandler(telegramService, cfg.APIKey)

	// Set up HTTP routes
	http.HandleFunc("/health", webhookHandler.HandleHealth)
	http.HandleFunc("/webhook", webhookHandler.HandleWebhook)

	// Start server
	log.Printf("Server starting on port %s", cfg.ServerPort)
	log.Fatal(http.ListenAndServe(":"+cfg.ServerPort, nil))
}
