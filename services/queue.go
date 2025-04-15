package services

import (
	"fmt"
	"log"
	"time"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
)

type QueueService struct {
	messages chan *models.QueuedMessage
	telegram *TelegramService
}

func NewQueueService(telegram *TelegramService) *QueueService {
	queue := &QueueService{
		messages: make(chan *models.QueuedMessage, 100),
		telegram: telegram,
	}
	go queue.processMessages()
	return queue
}

func (q *QueueService) EnqueueMessage(req *models.WebhookRequest) error {
	// Validate message size
	if len(req.Message) > models.MaxMessageSize {
		return fmt.Errorf("message exceeds maximum size of %d bytes", models.MaxMessageSize)
	}

	queued := &models.QueuedMessage{
		WebhookRequest: *req,
		Status:         models.StatusPending,
		RetryCount:     0,
		LastAttempt:    time.Now(),
	}

	q.messages <- queued
	return nil
}

func (q *QueueService) processMessages() {
	for msg := range q.messages {
		if msg.RetryCount >= models.MaxRetries {
			msg.Status = models.StatusFailed
			log.Printf("Message failed after %d retries: %v", models.MaxRetries, msg.Error)
			continue
		}

		msg.Status = models.StatusRetrying
		err := q.telegram.SendMessage(&msg.WebhookRequest)
		if err != nil {
			msg.Error = err.Error()
			msg.RetryCount++
			time.Sleep(models.RetryDelay)
			q.messages <- msg
			continue
		}

		msg.Status = models.StatusSent
		log.Printf("Message successfully sent to chat %d", msg.ChatID)
	}
}
