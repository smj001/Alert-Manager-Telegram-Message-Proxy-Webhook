package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
)

const maxTelegramReply = 64 << 10

type TelegramService struct {
	endpoint string
	client   *http.Client
}

func NewTelegramService(botToken string) *TelegramService {
	return &TelegramService{
		endpoint: "https://api.telegram.org/bot" + botToken + "/",
		client: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// deliveryError deliberately excludes Telegram's response body and network
// error text, which can contain message content or the token-bearing URL.
type deliveryError struct {
	status     int
	retryable  bool
	retryAfter time.Duration
}

func (e *deliveryError) Error() string {
	if e.status == 0 {
		return "Telegram transport failed or timed out"
	}
	return fmt.Sprintf("Telegram returned an unsuccessful response (status %d)", e.status)
}

// Send issues one Telegram call. For media it passes the URL to Telegram;
// the relay does not download the attachment itself.
func (s *TelegramService) Send(ctx context.Context, job models.WebhookRequest) error {
	method := "sendMessage"
	payload := map[string]any{"chat_id": job.ChatID}
	if job.MediaURL == "" {
		payload["text"] = job.Message
		if job.SuppressLinkPreview {
			payload["link_preview_options"] = map[string]bool{"is_disabled": true}
		}
	} else {
		switch job.MediaType {
		case "photo":
			method = "sendPhoto"
		case "video":
			method = "sendVideo"
		case "document":
			method = "sendDocument"
		default:
			return errors.New("unsupported Telegram media type")
		}
		payload[job.MediaType] = job.MediaURL
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Telegram request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint+method, bytes.NewReader(body))
	if err != nil {
		return errors.New("could not construct Telegram request")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return &deliveryError{retryable: true}
	}
	defer func() { _ = response.Body.Close() }()
	return checkTelegramResponse(response)
}

func checkTelegramResponse(response *http.Response) error {
	var reply struct {
		OK         bool `json:"ok"`
		ErrorCode  int  `json:"error_code"`
		Parameters struct {
			RetryAfter int64 `json:"retry_after"`
		} `json:"parameters"`
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxTelegramReply+1))
	parsed := err == nil && len(data) <= maxTelegramReply && json.Unmarshal(data, &reply) == nil
	status := response.StatusCode
	if status >= 200 && status < 300 {
		if parsed && reply.OK {
			return nil
		}
		if parsed && reply.ErrorCode >= 400 && reply.ErrorCode < 600 {
			status = reply.ErrorCode
		} else {
			return &deliveryError{status: status, retryable: true}
		}
	}
	var wait time.Duration
	if seconds := reply.Parameters.RetryAfter; parsed && status == http.StatusTooManyRequests && seconds > 0 && seconds <= (1<<63-1)/int64(time.Second) {
		wait = time.Duration(seconds) * time.Second
	}
	return &deliveryError{status: status, retryable: status == http.StatusTooManyRequests || status >= 500, retryAfter: wait}
}
