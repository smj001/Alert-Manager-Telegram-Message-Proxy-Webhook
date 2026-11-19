package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
)

func TestTelegramOperations(t *testing.T) {
	for _, tt := range []struct {
		name, mediaType, operation, field string
		suppressLinkPreview               bool
	}{
		{"general text", "", "sendMessage", "text", false},
		{"alert digest", "", "sendMessage", "text", true},
		{"photo", "photo", "sendPhoto", "photo", false},
		{"video", "video", "sendVideo", "video", false},
		{"document", "document", "sendDocument", "document", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			job := models.WebhookRequest{ChatID: -123, Message: "hello", MediaType: tt.mediaType, SuppressLinkPreview: tt.suppressLinkPreview}
			value := job.Message
			if tt.mediaType != "" {
				job.Message, job.MediaURL = "", "https://example.com/file"
				value = job.MediaURL
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.Method != http.MethodPost || r.URL.Path != "/bot123:test/"+tt.operation || body[tt.field] != value || body["chat_id"] != float64(-123) {
					t.Errorf("unexpected Telegram request: %s %s %v %v", r.Method, r.URL.Path, body, err)
				}
				if options, ok := body["link_preview_options"]; tt.suppressLinkPreview {
					if preview, ok := options.(map[string]any); !ok || preview["is_disabled"] != true {
						t.Errorf("expected explicitly disabled preview, got %v", options)
					}
				} else if ok {
					t.Errorf("unexpected preview option: %v", options)
				}
				if _, ok := body["parse_mode"]; ok {
					t.Error("alert text must not be interpreted as markup")
				}
				_, _ = fmt.Fprint(w, `{"ok":true}`)
			}))
			defer server.Close()
			bot := NewTelegramService("123:test")
			bot.endpoint = server.URL + "/bot123:test/"
			if err := bot.Send(context.Background(), job); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTelegramErrorClassification(t *testing.T) {
	for _, tt := range []struct {
		status    int
		body      string
		retryable bool
		wait      time.Duration
	}{
		{400, `{"ok":false,"error_code":400,"description":"do not log 123:secret"}`, false, 0},
		{429, `{"ok":false,"error_code":429,"parameters":{"retry_after":7}}`, true, 7 * time.Second},
		{503, `not JSON`, true, 0},
		{200, `not JSON`, true, 0},
		{302, `redirect`, false, 0},
	} {
		t.Run(fmt.Sprint(tt.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/bot123:secret/sendMessage" {
					t.Errorf("followed redirect to %s", r.URL.Path)
				}
				w.Header().Set("Location", "/redirect")
				w.WriteHeader(tt.status)
				_, _ = fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			bot := NewTelegramService("123:secret")
			bot.endpoint = server.URL + "/bot123:secret/"
			err := bot.Send(context.Background(), models.WebhookRequest{ChatID: 1, Message: "hello"})
			var delivery *deliveryError
			if !errors.As(err, &delivery) || delivery.retryable != tt.retryable || delivery.retryAfter != tt.wait || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unexpected Telegram error: %#v", err)
			}
		})
	}
}
