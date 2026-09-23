package app

import (
	"testing"

	"github.com/qobulov/brothers-app/pkg/config"
)

func TestShouldStartTelegramPolling(t *testing.T) {
	tests := []struct {
		name          string
		vercel        string
		token         string
		webhookSecret string
		want          bool
	}{
		{name: "local persistent process polls", token: "bot-token", want: true},
		{name: "missing bot token does not poll", want: false},
		{name: "configured webhook does not poll", token: "bot-token", webhookSecret: "webhook-secret", want: false},
		{name: "Vercel does not poll", vercel: "1", token: "bot-token", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("VERCEL", test.vercel)
			cfg := &config.Config{
				TelegramBotToken:      test.token,
				TelegramWebhookSecret: test.webhookSecret,
			}

			if got := shouldStartTelegramPolling(cfg); got != test.want {
				t.Fatalf("shouldStartTelegramPolling() = %t, want %t", got, test.want)
			}
		})
	}
}
