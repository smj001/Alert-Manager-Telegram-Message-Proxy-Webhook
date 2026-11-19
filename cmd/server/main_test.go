package main

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/services"
)

func TestServeRoutesAndDrain(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	deliveries := make(chan models.WebhookRequest, 2)
	queue := services.NewQueueService(func(_ context.Context, job models.WebhookRequest) error {
		deliveries <- job
		return nil
	})
	done := make(chan error, 1)
	go func() { done <- serve(ctx, listener, queue, "test-key") }()
	client := &http.Client{Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	for _, tt := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/health", "", 200},
		{"POST", "/webhook", `{"chat_id":1,"message":"hello"}`, 202},
		{"POST", "/alertmanager/webhook", `{"alerts":[{"status":"resolved","labels":{"chat_id":"-2","alertname":"Demo"}}]}`, 202},
	} {
		req, err := http.NewRequest(tt.method, "http://"+listener.Addr().String()+tt.path, strings.NewReader(tt.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer test-key")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != tt.status {
			t.Fatalf("%s: got %d, want %d", tt.path, resp.StatusCode, tt.status)
		}
	}
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not stop")
	}
	close(deliveries)
	received := map[int64]string{}
	for job := range deliveries {
		received[job.ChatID] = job.Message
	}
	if len(received) != 2 || received[1] != "hello" || !strings.Contains(received[-2], "RESOLVED") {
		t.Fatalf("unexpected deliveries: %v", received)
	}
}
