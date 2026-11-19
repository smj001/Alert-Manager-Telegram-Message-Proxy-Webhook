package services

import (
	"container/list"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
)

const (
	workerCount  = 20
	queueSize    = 5000
	sendInterval = 2 * time.Second // 30 Telegram calls per minute, per process.
	retryDelay   = 5 * time.Second
	maxAttempts  = 3
)

var ErrQueueUnavailable = errors.New("message queue is full or closed")

type QueueService struct {
	mu            sync.Mutex // Protects admission, ready groups, pending count, and cooldown.
	ready         *sync.Cond
	groups        list.List // Each entry holds the remaining pages of one chat's digest.
	closed        bool
	cooldownUntil time.Time
	capacity      int
	pending       int64
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
	ticks         <-chan time.Time
	delay         time.Duration
	send          func(context.Context, models.WebhookRequest) error
}

func NewQueueService(send func(context.Context, models.WebhookRequest) error) *QueueService {
	return newQueue(send, workerCount, queueSize, sendInterval, retryDelay)
}

func newQueue(send func(context.Context, models.WebhookRequest) error, workers, capacity int, interval, delay time.Duration) *QueueService {
	ctx, cancel := context.WithCancel(context.Background())
	ticker := time.NewTicker(interval)
	q := &QueueService{
		capacity: capacity, ctx: ctx, cancel: cancel,
		done: make(chan struct{}), ticks: ticker.C, delay: delay, send: send,
	}
	q.ready = sync.NewCond(&q.mu)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.worker()
		}()
	}
	go func() {
		wg.Wait()
		ticker.Stop()
		cancel()
		close(q.done)
	}()
	return q
}

// Enqueue admits the whole batch or none. Each message counts toward capacity,
// including pages waiting behind an in-flight message for the same chat.
func (q *QueueService) Enqueue(batch []models.WebhookRequest) error {
	if len(batch) > q.capacity {
		return ErrQueueUnavailable
	}
	groups := groupMessagesByChat(batch)
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || q.pending+int64(len(batch)) > int64(q.capacity) {
		return ErrQueueUnavailable
	}
	q.pending += int64(len(batch))
	for _, group := range groups {
		q.groups.PushBack(group)
	}
	q.ready.Broadcast()
	return nil
}

func groupMessagesByChat(batch []models.WebhookRequest) [][]models.WebhookRequest {
	groups := make([][]models.WebhookRequest, 0)
	byChat := make(map[int64]int)
	for _, message := range batch {
		index, ok := byChat[message.ChatID]
		if !ok {
			index = len(groups)
			byChat[message.ChatID] = index
			groups = append(groups, nil)
		}
		groups[index] = append(groups[index], message)
	}
	return groups
}

func (q *QueueService) worker() {
	for {
		group := q.nextGroup()
		if len(group) == 0 || !q.sendJob(group[0]) {
			return
		}
		// Release the completed page and let other groups take a turn before
		// sending the next page. Only one worker owns a group at a time.
		group[0] = models.WebhookRequest{}
		q.mu.Lock()
		q.pending--
		if len(group) > 1 {
			q.groups.PushBack(group[1:])
			q.ready.Signal()
		} else if q.closed && q.pending == 0 {
			q.ready.Broadcast()
		}
		q.mu.Unlock()
	}
}

func (q *QueueService) nextGroup() []models.WebhookRequest {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.ctx.Err() == nil {
		if head := q.groups.Front(); head != nil {
			return q.groups.Remove(head).([]models.WebhookRequest)
		}
		if q.closed && q.pending == 0 {
			return nil
		}
		q.ready.Wait()
	}
	return nil
}

func (q *QueueService) sendJob(job models.WebhookRequest) bool {
	if strings.TrimSpace(job.Message) != "" {
		text := job
		text.MediaType, text.MediaURL = "", ""
		if !q.deliver(text, "text") {
			return false
		}
	}
	if job.MediaURL != "" {
		job.Message = ""
		return q.deliver(job, job.MediaType)
	}
	return true
}

// deliver is complete when the operation succeeded or reached terminal failure.
// A false result means shutdown interrupted the operation.
func (q *QueueService) deliver(job models.WebhookRequest, kind string) bool {
	for attempt := 1; ; attempt++ {
		if !q.waitTurn() {
			return false
		}
		err := q.send(q.ctx, job)
		if err == nil {
			slog.Debug("Telegram delivery succeeded", "kind", kind)
			return true
		}
		if q.ctx.Err() != nil {
			return false
		}
		var failure *deliveryError
		known := errors.As(err, &failure)
		wait := q.delay
		if known && failure.status == 429 {
			wait = max(wait, failure.retryAfter)
			until := time.Now().Add(wait)
			q.mu.Lock()
			if until.After(q.cooldownUntil) {
				q.cooldownUntil = until
			}
			q.mu.Unlock()
		}
		if !known || !failure.retryable || attempt == maxAttempts {
			slog.Error("Telegram delivery failed", "kind", kind, "attempts", attempt, "error", err)
			return true
		}

		slog.Warn("Retrying Telegram delivery", "kind", kind, "attempt", attempt, "delay", wait, "error", err)
		if !q.wait(wait) {
			return false
		}
	}
}

func (q *QueueService) waitTurn() bool {
	for {
		select {
		case <-q.ctx.Done():
			return false
		case <-q.ticks:
		}
		q.mu.Lock()
		wait := time.Until(q.cooldownUntil)
		q.mu.Unlock()
		if wait <= 0 {
			return q.ctx.Err() == nil
		}
		if !q.wait(wait) {
			return false
		}
	}
}

func (q *QueueService) wait(delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-q.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Shutdown stops admission, drains until ctx expires, and returns the number
// of jobs that did not reach a terminal result, including interrupted sends.
func (q *QueueService) Shutdown(ctx context.Context) int64 {
	q.mu.Lock()
	q.closed = true
	q.ready.Broadcast()
	q.mu.Unlock()
	select {
	case <-q.done:
	case <-ctx.Done():
		q.cancel()
		q.mu.Lock()
		q.ready.Broadcast()
		q.mu.Unlock()
		<-q.done // All outbound requests and retry waits respect q.ctx.
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.pending
}
