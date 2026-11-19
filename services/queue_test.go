package services

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
)

func TestQueueDrainsWithoutRepeatingSuccessfulText(t *testing.T) {
	var attempts []string
	q := newQueue(func(_ context.Context, job models.WebhookRequest) error {
		if job.MediaURL == "" {
			attempts = append(attempts, "text")
			return nil
		}
		attempts = append(attempts, job.MediaType)
		if len(attempts) == 2 {
			return &deliveryError{retryable: true}
		}
		return nil
	}, 1, 2, time.Millisecond, time.Millisecond)
	job := models.WebhookRequest{ChatID: 1, Message: "hello", MediaType: "photo", MediaURL: "https://example.com/photo"}
	if err := q.Enqueue([]models.WebhookRequest{job}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if unfinished := q.Shutdown(ctx); unfinished != 0 {
		t.Fatalf("%d unfinished jobs", unfinished)
	}
	if !reflect.DeepEqual(attempts, []string{"text", "photo", "photo"}) {
		t.Fatalf("delivery attempts: %v", attempts)
	}
}

func TestQueueCapacityAndConcurrentShutdown(t *testing.T) {
	started := make(chan struct{})
	q := newQueue(func(ctx context.Context, _ models.WebhookRequest) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}, 1, 3, time.Millisecond, time.Millisecond)
	job := models.WebhookRequest{ChatID: 1, Message: "hello"}
	if err := q.Enqueue([]models.WebhookRequest{job}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start")
	}
	if err := q.Enqueue([]models.WebhookRequest{job}); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue([]models.WebhookRequest{job, job}); !errors.Is(err, ErrQueueUnavailable) {
		t.Fatalf("oversized batch: %v", err)
	}
	var wg sync.WaitGroup
	var admitted atomic.Int64
	racing := make(chan struct{})
	for range [8]struct{}{} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-racing
			if err := q.Enqueue([]models.WebhookRequest{job}); err == nil {
				admitted.Add(1)
			} else if !errors.Is(err, ErrQueueUnavailable) {
				t.Errorf("concurrent admission: %v", err)
			}
		}()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	close(racing)
	unfinished := q.Shutdown(ctx)
	wg.Wait()
	if want := 2 + admitted.Load(); unfinished != want || want > 3 {
		t.Fatalf("unfinished=%d, want %d (no partial batch admission)", unfinished, want)
	}
	if again := q.Shutdown(ctx); again != unfinished {
		t.Fatalf("second shutdown: unfinished=%d, want %d", again, unfinished)
	}
	if err := q.Enqueue([]models.WebhookRequest{job}); !errors.Is(err, ErrQueueUnavailable) {
		t.Fatalf("enqueue after shutdown: %v", err)
	}
}

func TestQueueSerializesChatPagesWithoutBlockingOtherChats(t *testing.T) {
	attempts := make(chan string, 10)
	otherChat := make(chan struct{})
	var firstPageCompleted atomic.Bool
	var secondPageAttempts int // Accessed by the worker handling chat 1's pages, then after drain.
	q := newQueue(func(ctx context.Context, job models.WebhookRequest) error {
		attempts <- job.Message
		switch job.Message {
		case "page 1":
			select {
			case <-otherChat:
				firstPageCompleted.Store(true)
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		case "page 2":
			if !firstPageCompleted.Load() {
				attempts <- "page 2 before page 1 completed"
			}
			secondPageAttempts++
			if secondPageAttempts == 1 {
				return &deliveryError{retryable: true}
			}
		case "other chat":
			close(otherChat)
		}
		return nil
	}, 2, 4, time.Millisecond, time.Millisecond)
	batch := []models.WebhookRequest{
		{ChatID: 1, Message: "page 1"}, {ChatID: 1, Message: "page 2"}, {ChatID: 1, Message: "page 3"}, {ChatID: 2, Message: "other chat"},
	}
	if err := q.Enqueue(batch); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if unfinished := q.Shutdown(ctx); unfinished != 0 {
		t.Fatalf("unfinished=%d", unfinished)
	}
	close(attempts)
	var got []string
	for message := range attempts {
		if message != "other chat" {
			got = append(got, message)
		}
	}
	if !reflect.DeepEqual(got, []string{"page 1", "page 2", "page 2", "page 3"}) {
		t.Fatalf("attempts out of order: %v", got)
	}
}

func TestQueueYieldsBetweenPages(t *testing.T) {
	var received []string
	q := newQueue(func(_ context.Context, job models.WebhookRequest) error {
		received = append(received, job.Message)
		return nil
	}, 1, 4, time.Millisecond, time.Millisecond)
	if err := q.Enqueue([]models.WebhookRequest{
		{ChatID: 1, Message: "page 1"}, {ChatID: 1, Message: "page 2"}, {ChatID: 2, Message: "short notification"},
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if unfinished := q.Shutdown(ctx); unfinished != 0 {
		t.Fatalf("unfinished=%d", unfinished)
	}
	if !reflect.DeepEqual(received, []string{"page 1", "short notification", "page 2"}) {
		t.Fatalf("queued work blocked by a long digest: %v", received)
	}
}

func TestQueueCountsUnfinishedDigestPages(t *testing.T) {
	blocked := make(chan struct{})
	q := newQueue(func(ctx context.Context, job models.WebhookRequest) error {
		if job.Message == "page 2" {
			close(blocked)
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}, 1, 3, time.Millisecond, time.Millisecond)
	if err := q.Enqueue([]models.WebhookRequest{
		{ChatID: 1, Message: "page 1"}, {ChatID: 1, Message: "page 2"}, {ChatID: 1, Message: "page 3"},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("page 2 did not start")
	}
	other := models.WebhookRequest{ChatID: 2, Message: "other chat"}
	if err := q.Enqueue([]models.WebhookRequest{other, other}); !errors.Is(err, ErrQueueUnavailable) {
		t.Fatalf("expected capacity rejection, got %v", err)
	}
	if err := q.Enqueue([]models.WebhookRequest{other}); err != nil {
		t.Fatalf("completed page did not free space: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if unfinished := q.Shutdown(ctx); unfinished != 3 {
		t.Fatalf("unfinished=%d, want 3", unfinished)
	}
}

func TestQueueDrainWakesIdleWorkersAfterFinalPage(t *testing.T) {
	sending := make(chan struct{})
	finish := make(chan struct{})
	q := newQueue(func(ctx context.Context, _ models.WebhookRequest) error {
		close(sending)
		select {
		case <-finish:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, 2, 2, time.Millisecond, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		q.Shutdown(ctx)
	}()
	if err := q.Enqueue([]models.WebhookRequest{{ChatID: 1, Message: "final page"}}); err != nil {
		t.Fatal(err)
	}
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	select {
	case <-sending:
	case <-timeout.C:
		t.Fatal("last page did not start")
	}
	drained := make(chan int64, 1)
	go func() { drained <- q.Shutdown(ctx) }()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !errors.Is(q.Enqueue(nil), ErrQueueUnavailable) {
		select {
		case <-ticker.C:
		case <-timeout.C:
			t.Fatal("drain did not start")
		}
	}
	close(finish)
	select {
	case unfinished := <-drained:
		if unfinished != 0 {
			t.Fatalf("unfinished=%d", unfinished)
		}
	case <-timeout.C:
		t.Fatal("idle workers did not exit after the final page")
	}
}

func TestQueueSharesTelegramCooldown(t *testing.T) {
	const cooldown = 60 * time.Millisecond
	times := make(chan time.Time, 3)
	var calls atomic.Int32
	q := newQueue(func(context.Context, models.WebhookRequest) error {
		times <- time.Now()
		if calls.Add(1) == 1 {
			return &deliveryError{status: 429, retryable: true, retryAfter: cooldown}
		}
		return nil
	}, 2, 2, 15*time.Millisecond, time.Millisecond)
	if err := q.Enqueue([]models.WebhookRequest{{ChatID: 1, Message: "alert"}, {ChatID: 2, Message: "alert"}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if unfinished := q.Shutdown(ctx); unfinished != 0 || len(times) != 3 {
		t.Fatalf("attempts=%d unfinished=%d", len(times), unfinished)
	}
	first, second := <-times, <-times
	if second.Sub(first) < cooldown {
		t.Fatalf("a worker sent before the shared cooldown elapsed: %s", second.Sub(first))
	}
}

func TestQueueRetryLimits(t *testing.T) {
	for _, tt := range []struct {
		name     string
		err      error
		attempts int
	}{
		{"permanent", &deliveryError{status: 400}, 1},
		{"temporary", &deliveryError{status: 503, retryable: true}, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			q := newQueue(func(context.Context, models.WebhookRequest) error {
				attempts++
				return tt.err
			}, 1, 2, time.Millisecond, time.Millisecond)
			if err := q.Enqueue([]models.WebhookRequest{{ChatID: 2, Message: "alert"}}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if unfinished := q.Shutdown(ctx); unfinished != 0 || attempts != tt.attempts {
				t.Fatalf("attempts=%d unfinished=%d; wanted %d and 0", attempts, unfinished, tt.attempts)
			}
		})
	}
}
