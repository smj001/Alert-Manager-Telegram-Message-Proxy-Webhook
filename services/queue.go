package services

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
)

type QueueService struct {
	messages    chan *models.QueuedMessage
	telegram    *TelegramService
	workerCount int
	wg          sync.WaitGroup
	rateLimiter chan struct{}
	mu          sync.Mutex
	stats       struct {
		processed int
		failed    int
		queued    int
	}
}

const (
	WorkerCount     = 20          // Increased worker count
	RateLimit       = 30          // Messages per minute
	RateLimitWindow = time.Minute // Rate limit window
	QueueSize       = 5000        // Increased queue size
)

func NewQueueService(telegram *TelegramService) *QueueService {
	queue := &QueueService{
		messages:    make(chan *models.QueuedMessage, QueueSize),
		telegram:    telegram,
		workerCount: WorkerCount,
		rateLimiter: make(chan struct{}, RateLimit),
	}

	// Initialize rate limiter
	go func() {
		ticker := time.NewTicker(RateLimitWindow / RateLimit)
		for range ticker.C {
			select {
			case queue.rateLimiter <- struct{}{}:
			default:
			}
		}
	}()

	// Start worker pool
	for i := 0; i < queue.workerCount; i++ {
		queue.wg.Add(1)
		go queue.worker()
	}

	// Start stats logging
	go queue.logStats()

	return queue
}

func (q *QueueService) logStats() {
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		q.mu.Lock()
		log.Printf("Queue Stats - Processed: %d, Failed: %d, Queued: %d",
			q.stats.processed, q.stats.failed, q.stats.queued)
		q.mu.Unlock()
	}
}

func (q *QueueService) worker() {
	defer q.wg.Done()
	for msg := range q.messages {
		q.processMessage(msg)
	}
}

func (q *QueueService) processMessage(msg *models.QueuedMessage) {
	if msg.RetryCount >= models.MaxRetries {
		msg.Status = models.StatusFailed
		q.mu.Lock()
		q.stats.failed++
		q.mu.Unlock()
		log.Printf("Message failed after %d retries: %v", models.MaxRetries, msg.Error)
		return
	}

	// Wait for rate limiter
	<-q.rateLimiter

	msg.Status = models.StatusRetrying
	err := q.telegram.SendMessage(&msg.WebhookRequest)
	if err != nil {
		msg.Error = err.Error()
		msg.RetryCount++
		time.Sleep(models.RetryDelay)
		q.messages <- msg
		return
	}

	msg.Status = models.StatusSent
	q.mu.Lock()
	q.stats.processed++
	q.mu.Unlock()
	log.Printf("Message successfully sent to chat %d", msg.ChatID)
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

	select {
	case q.messages <- queued:
		q.mu.Lock()
		q.stats.queued++
		q.mu.Unlock()
		return nil
	default:
		return fmt.Errorf("message queue is full, please try again later")
	}
}

func (q *QueueService) Shutdown() {
	close(q.messages)
	q.wg.Wait()
}
