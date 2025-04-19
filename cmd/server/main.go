package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/config"
	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/handlers"
	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/pkg/logger"
	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/services"
)

func main() {
	// Initialize logger
	log := logger.New()
	log.Info("Starting server...")

	// Load configuration
	cfg := config.LoadConfig()
	log.Info("Configuration loaded successfully")

	// Initialize services
	telegramService, err := services.NewTelegramService(cfg.BotToken)
	if err != nil {
		log.Fatal("Failed to initialize Telegram service: %v", err)
	}

	queueService := services.NewQueueService(telegramService)

	// Initialize handlers
	webhookHandler := handlers.NewWebhookHandler(telegramService, queueService, cfg.APIKey)
	alertManagerHandler := handlers.NewAlertManagerHandler(telegramService, queueService, cfg.APIKey)

	// Set up routes
	http.HandleFunc("/webhook", webhookHandler.HandleWebhook)
	http.HandleFunc("/alertmanager/webhook", alertManagerHandler.HandleAlertManagerWebhook)
	http.HandleFunc("/health", webhookHandler.HandleHealth)

	// Set up graceful shutdown
	server := &http.Server{
		Addr: fmt.Sprintf(":%s", cfg.ServerPort),
	}

	// Channel to listen for errors coming from the listener.
	serverErrors := make(chan error, 1)

	// Start the server
	go func() {
		log.Info("Server listening on port %s", cfg.ServerPort)
		serverErrors <- server.ListenAndServe()
	}()

	// Channel to listen for an interrupt or terminate signal from the OS.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Blocking main and waiting for shutdown.
	select {
	case err := <-serverErrors:
		log.Error("Error starting server: %v", err)
		os.Exit(1)

	case sig := <-shutdown:
		log.Info("Shutdown signal received: %v", sig)
		// TODO: Add graceful shutdown logic here
		os.Exit(0)
	}
}
