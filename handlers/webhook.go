package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/services"
)

type WebhookHandler struct {
	telegramService *services.TelegramService
	queueService    *services.QueueService
	apiKey          string
}

func NewWebhookHandler(telegramService *services.TelegramService, apiKey string) *WebhookHandler {
	queueService := services.NewQueueService(telegramService)
	return &WebhookHandler{
		telegramService: telegramService,
		queueService:    queueService,
		apiKey:          apiKey,
	}
}

func (h *WebhookHandler) authenticateRequest(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	return authHeader == fmt.Sprintf("Bearer %s", h.apiKey)
}

func (h *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.authenticateRequest(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.WebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.queueService.EnqueueMessage(&req); err != nil {
		http.Error(w, fmt.Sprintf("Failed to queue message: %v", err), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(models.WebhookResponse{Status: "queued"})
}

func (h *WebhookHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}
