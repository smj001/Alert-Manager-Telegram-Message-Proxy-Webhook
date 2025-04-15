package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/services"
)

type AlertManagerHandler struct {
	telegramService *services.TelegramService
	queueService    *services.QueueService
	apiKey          string
}

func NewAlertManagerHandler(telegramService *services.TelegramService, queueService *services.QueueService, apiKey string) *AlertManagerHandler {
	return &AlertManagerHandler{
		telegramService: telegramService,
		queueService:    queueService,
		apiKey:          apiKey,
	}
}

func (h *AlertManagerHandler) authenticateRequest(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	return authHeader == fmt.Sprintf("Bearer %s", h.apiKey)
}

func (h *AlertManagerHandler) formatAlertMessage(alert models.AlertManagerAlert) string {
	var sb strings.Builder

	sb.WriteString("\n\nAlerts Firing:\n")
	sb.WriteString("Labels:\n")
	for k, v := range alert.Labels {
		sb.WriteString(fmt.Sprintf(" - %s = %s\n", k, v))
	}

	sb.WriteString("Annotations:\n")
	for k, v := range alert.Annotations {
		sb.WriteString(fmt.Sprintf(" - %s = %s\n", k, v))
	}

	return sb.String()
}

func (h *AlertManagerHandler) HandleAlertManager(w http.ResponseWriter, r *http.Request) {
	if !h.authenticateRequest(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.AlertManagerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Process each alert
	for _, alert := range req.Alerts {
		message := h.formatAlertMessage(alert)
		webhookReq := &models.WebhookRequest{
			ChatID:  -1002675286276, // Your default chat ID
			Message: message,
		}

		if err := h.queueService.EnqueueMessage(webhookReq); err != nil {
			http.Error(w, fmt.Sprintf("Failed to queue message: %v", err), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(models.WebhookResponse{Status: "queued"})
}
