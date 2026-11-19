package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/config"
	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/handlers"
	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/services"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// Restore the default behavior after the first signal so a second one can
	// interrupt the drain in an emergency.
	go func() {
		<-ctx.Done()
		stop()
	}()
	err := run(ctx)
	stop()
	if err != nil {
		slog.Error("Server stopped with an error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})))
	listener, err := net.Listen("tcp", ":"+cfg.ServerPort)
	if err != nil {
		return fmt.Errorf("open HTTP listener: %w", err)
	}
	telegram := services.NewTelegramService(cfg.BotToken)
	queue := services.NewQueueService(telegram.Send)
	return serve(ctx, listener, queue, cfg.APIKey)
}

type drainingQueue interface {
	handlers.MessageQueue
	Shutdown(context.Context) int64
}

func serve(ctx context.Context, listener net.Listener, queue drainingQueue, apiKey string) error {
	webhook := handlers.NewWebhookHandler(queue, apiKey)
	alertmanager := handlers.NewAlertManagerHandler(queue, apiKey)
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", webhook.HandleWebhook)
	mux.HandleFunc("/alertmanager/webhook", alertmanager.HandleAlertManagerWebhook)
	mux.HandleFunc("/health", webhook.HandleHealth)
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       8 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	slog.Info("Server listening", "address", listener.Addr())
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(listener) }()

	var serveErr error
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			serveErr = fmt.Errorf("serve HTTP: %w", err)
		}
	case <-ctx.Done():
		slog.Info("Shutdown requested; stopping HTTP intake")
	}

	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), 10*time.Second)
	if err := server.Shutdown(httpCtx); err != nil {
		serveErr = errors.Join(serveErr, fmt.Errorf("stop HTTP server: %w", err), server.Close())
	}
	cancelHTTP()

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelDrain()
	if unfinished := queue.Shutdown(drainCtx); unfinished > 0 {
		return errors.Join(serveErr, fmt.Errorf("shutdown deadline: %d accepted jobs did not finish", unfinished))
	}
	slog.Info("Queue drained")
	return serveErr
}
