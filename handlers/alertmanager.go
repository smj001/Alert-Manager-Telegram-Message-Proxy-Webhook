package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
)

type AlertManagerHandler struct {
	queue  MessageQueue
	apiKey string
}

func NewAlertManagerHandler(queue MessageQueue, apiKey string) *AlertManagerHandler {
	return &AlertManagerHandler{queue: queue, apiKey: apiKey}
}

func (h *AlertManagerHandler) HandleAlertManagerWebhook(w http.ResponseWriter, r *http.Request) {
	var alertManagerWebhook models.AlertManagerWebhook
	if !authorizeWebhook(w, r, h.apiKey) || !decodeWebhook(w, r, &alertManagerWebhook) {
		return
	}
	if len(alertManagerWebhook.Alerts) == 0 {
		http.Error(w, "Expected at least one alert", http.StatusBadRequest)
		return
	}

	var destinations []int64
	byChat := make(map[int64][]models.AlertManagerAlert)
	for _, alert := range alertManagerWebhook.Alerts {
		chatIDStr, ok := alert.Labels["chat_id"]
		if !ok {
			chatIDStr = alertManagerWebhook.CommonLabels["chat_id"]
		}
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil || chatID == 0 {
			http.Error(w, "Each alert requires a nonzero numeric chat_id", http.StatusBadRequest)
			return
		}
		if alert.Status != "firing" && alert.Status != "resolved" {
			http.Error(w, "Each alert requires a firing or resolved status", http.StatusBadRequest)
			return
		}
		if _, seen := byChat[chatID]; !seen {
			destinations = append(destinations, chatID)
		}
		byChat[chatID] = append(byChat[chatID], alert)
	}
	if alertManagerWebhook.TruncatedAlerts > 0 {
		slog.Warn("Alertmanager truncated the incoming group", "omitted_alerts", alertManagerWebhook.TruncatedAlerts)
	}
	batch := make([]models.WebhookRequest, 0, len(destinations))
	for _, chatID := range destinations {
		for _, page := range digestPages(byChat[chatID]) {
			batch = append(batch, models.WebhookRequest{ChatID: chatID, Message: page.text, Entities: page.entities, SuppressLinkPreview: true})
		}
	}
	acceptBatch(w, h.queue, batch)
}
