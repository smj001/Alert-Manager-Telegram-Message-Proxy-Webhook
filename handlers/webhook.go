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
	apiKey          string
}

func NewWebhookHandler(telegramService *services.TelegramService, apiKey string) *WebhookHandler {
	return &WebhookHandler{
		telegramService: telegramService,
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

	if err := h.telegramService.SendMessage(&req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(models.WebhookResponse{Status: "success"})
}
