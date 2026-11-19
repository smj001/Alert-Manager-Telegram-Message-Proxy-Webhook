package config

import (
	"log/slog"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	for _, tt := range []struct {
		name, token, key, port, level, wantError string
	}{
		{"defaults", "123:fake-token", "fake-key", "", "", ""},
		{"valid overrides", "123:fake-token", "fake-key", "3000", "debug", ""},
		{"no token", "", "fake-key", "", "", "TELEGRAM_BOT_TOKEN"},
		{"invalid token", "123:fake/token", "fake-key", "", "", "TELEGRAM_BOT_TOKEN"},
		{"no key", "123:fake-token", "", "", "", "WEBHOOK_API_KEY"},
		{"placeholder key", "123:fake-token", "your_api_key_here", "", "", "WEBHOOK_API_KEY"},
		{"zero port", "123:fake-token", "fake-key", "0", "", "SERVER_PORT"},
		{"out of range", "123:fake-token", "fake-key", "65536", "", "SERVER_PORT"},
		{"non-numeric port", "123:fake-token", "fake-key", "http", "", "SERVER_PORT"},
		{"invalid log level", "123:fake-token", "fake-key", "", "trace", "LOG_LEVEL"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TELEGRAM_BOT_TOKEN", tt.token)
			t.Setenv("WEBHOOK_API_KEY", tt.key)
			t.Setenv("SERVER_PORT", tt.port)
			t.Setenv("LOG_LEVEL", tt.level)
			cfg, err := LoadConfig()
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) || strings.Contains(err.Error(), "fake") {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			port, level := "8080", slog.LevelInfo
			if tt.port != "" {
				port, level = tt.port, slog.LevelDebug
			}
			if cfg.ServerPort != port || cfg.LogLevel != level || cfg.BotToken != tt.token || cfg.APIKey != tt.key {
				t.Fatal("configuration did not match expected values")
			}
		})
	}
}
