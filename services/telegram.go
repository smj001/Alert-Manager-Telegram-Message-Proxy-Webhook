package services

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook/models"
)

type TelegramService struct {
	bot *tgbotapi.BotAPI
}

func NewTelegramService(botToken string) (*TelegramService, error) {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize bot: %w", err)
	}

	return &TelegramService{bot: bot}, nil
}

func (s *TelegramService) SendMessage(req *models.WebhookRequest) error {
	// Send text message
	msg := tgbotapi.NewMessage(req.ChatID, req.Message)
	if _, err := s.bot.Send(msg); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// If media URL is provided, send media
	if req.MediaURL != "" {
		switch req.MediaType {
		case "photo":
			photo := tgbotapi.NewPhoto(req.ChatID, tgbotapi.FileURL(req.MediaURL))
			if _, err := s.bot.Send(photo); err != nil {
				return fmt.Errorf("failed to send photo: %w", err)
			}
		case "video":
			video := tgbotapi.NewVideo(req.ChatID, tgbotapi.FileURL(req.MediaURL))
			if _, err := s.bot.Send(video); err != nil {
				return fmt.Errorf("failed to send video: %w", err)
			}
		case "document":
			doc := tgbotapi.NewDocument(req.ChatID, tgbotapi.FileURL(req.MediaURL))
			if _, err := s.bot.Send(doc); err != nil {
				return fmt.Errorf("failed to send document: %w", err)
			}
		}
	}

	return nil
}
