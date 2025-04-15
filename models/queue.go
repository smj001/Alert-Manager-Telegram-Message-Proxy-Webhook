package models

import "time"

type MessageStatus string

const (
	StatusPending  MessageStatus = "pending"
	StatusSent     MessageStatus = "sent"
	StatusFailed   MessageStatus = "failed"
	StatusRetrying MessageStatus = "retrying"
)

type QueuedMessage struct {
	WebhookRequest
	Status      MessageStatus `json:"status"`
	RetryCount  int           `json:"retry_count"`
	LastAttempt time.Time     `json:"last_attempt"`
	Error       string        `json:"error,omitempty"`
}

const (
	MaxRetries     = 3
	RetryDelay     = 5 * time.Second
	MaxMessageSize = 4096             // Telegram's message size limit
	MaxMediaSize   = 50 * 1024 * 1024 // 50MB Telegram media limit
)
