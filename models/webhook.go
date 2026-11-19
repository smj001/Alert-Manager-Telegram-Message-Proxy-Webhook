package models

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	MaxMessageSize = 4096 // Unicode code points, not UTF-8 bytes.
	MaxAlertBatch  = 500
	maxMediaURL    = 4096
)

type WebhookRequest struct {
	ChatID    int64  `json:"chat_id"`
	Message   string `json:"message"`
	MediaURL  string `json:"media_url,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	// Internal rendering preference; not part of the public webhook payload.
	SuppressLinkPreview bool `json:"-"`
}

func (r WebhookRequest) Validate() error {
	if r.ChatID == 0 {
		return errors.New("chat_id must be nonzero")
	}
	if utf8.RuneCountInString(r.Message) > MaxMessageSize {
		return fmt.Errorf("message exceeds %d characters", MaxMessageSize)
	}
	if r.MediaURL == "" {
		if r.MediaType != "" {
			return errors.New("media_url is required with media_type")
		}
		if strings.TrimSpace(r.Message) == "" {
			return errors.New("message or media_url is required")
		}
		return nil
	}
	switch r.MediaType {
	case "photo", "video", "document":
	default:
		return errors.New("media_type must be photo, video, or document")
	}
	if len(r.MediaURL) > maxMediaURL {
		return fmt.Errorf("media_url exceeds %d bytes", maxMediaURL)
	}
	u, err := url.ParseRequestURI(r.MediaURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return errors.New("media_url must be an absolute HTTP(S) URL without credentials")
	}
	return nil
}

type WebhookResponse struct {
	Status string `json:"status"`
}
