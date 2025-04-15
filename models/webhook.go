package models

type WebhookRequest struct {
	ChatID    int64  `json:"chat_id"`
	Message   string `json:"message"`
	MediaURL  string `json:"media_url,omitempty"`
	MediaType string `json:"media_type,omitempty"`
}

type WebhookResponse struct {
	Status string `json:"status"`
}
