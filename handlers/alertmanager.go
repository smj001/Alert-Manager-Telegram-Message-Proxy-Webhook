package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

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

func (h *AlertManagerHandler) HandleAlertManagerWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.authenticateRequest(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var alertManagerWebhook models.AlertManagerWebhook
	if err := json.NewDecoder(r.Body).Decode(&alertManagerWebhook); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get chat_id from common labels
	chatIDStr, ok := alertManagerWebhook.CommonLabels["chat_id"]
	if !ok {
		http.Error(w, "chat_id not found in common labels", http.StatusBadRequest)
		return
	}

	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid chat_id format", http.StatusBadRequest)
		return
	}

	// Process each alert
	for _, alert := range alertManagerWebhook.Alerts {
		// Format the message
		message := formatAlertMessage(alert, alertManagerWebhook.CommonLabels)

		// Create webhook request
		req := &models.WebhookRequest{
			ChatID:  chatID,
			Message: message,
		}

		// Queue the message
		if err := h.queueService.EnqueueMessage(req); err != nil {
			http.Error(w, fmt.Sprintf("Failed to queue message: %v", err), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(models.WebhookResponse{Status: "queued"})
}

func formatAlertMessage(alert models.AlertManagerAlert, commonLabels map[string]string) string {
	status := "🔴 FIRING"
	if alert.Status == "resolved" {
		status = "✅ RESOLVED"
	}

	message := fmt.Sprintf("%s\n", status)
	message += fmt.Sprintf("Alert: %s\n", alert.Labels["alertname"])

	if severity, ok := alert.Labels["severity"]; ok {
		message += fmt.Sprintf("Severity: %s\n", severity)
	}

	if instance, ok := alert.Labels["instance"]; ok {
		message += fmt.Sprintf("Instance: %s\n", instance)
	}

	if description, ok := alert.Annotations["description"]; ok {
		message += fmt.Sprintf("Description: %s\n", description)
	}

	if summary, ok := alert.Annotations["summary"]; ok {
		message += fmt.Sprintf("Summary: %s\n", summary)
	}

	if alert.GeneratorURL != "" {
		message += fmt.Sprintf("Source: %s\n", alert.GeneratorURL)
	}

	return message
}
