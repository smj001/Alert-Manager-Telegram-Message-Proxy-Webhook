package handlers

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/services"
)

const maxBodyBytes = 1 << 20

type MessageQueue interface {
	Enqueue([]models.WebhookRequest) error
}

type WebhookHandler struct {
	queue  MessageQueue
	apiKey string
}

func NewWebhookHandler(queue MessageQueue, apiKey string) *WebhookHandler {
	return &WebhookHandler{queue: queue, apiKey: apiKey}
}

func authorizeWebhook(w http.ResponseWriter, r *http.Request, apiKey string) bool {
	scheme, key, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	expected, provided := sha256.Sum256([]byte(apiKey)), sha256.Sum256([]byte(key))
	if !ok || apiKey == "" || !strings.EqualFold(scheme, "Bearer") || subtle.ConstantTimeCompare(expected[:], provided[:]) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

func decodeWebhook(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.ContentLength > maxBodyBytes {
		http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	err := decoder.Decode(dst)
	if err == nil {
		// Only one JSON value is allowed; reading again also enforces the limit on trailing data.
		err = decoder.Decode(new(json.RawMessage))
		if err == io.EOF {
			return true
		}
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
	} else {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
	}
	return false
}

func acceptBatch(w http.ResponseWriter, queue MessageQueue, batch []models.WebhookRequest) {
	if err := queue.Enqueue(batch); err != nil {
		if errors.Is(err, services.ErrQueueUnavailable) {
			w.Header().Set("Retry-After", "5")
			http.Error(w, "Message queue unavailable", http.StatusServiceUnavailable)
		} else {
			http.Error(w, "Could not accept messages", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(models.WebhookResponse{Status: "queued"})
}

func (h *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	var req models.WebhookRequest
	if !authorizeWebhook(w, r, h.apiKey) || !decodeWebhook(w, r, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	acceptBatch(w, h.queue, []models.WebhookRequest{req})
}

func (h *WebhookHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}
