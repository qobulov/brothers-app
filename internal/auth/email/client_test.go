package email

import (
	"context"
	"strings"
	"testing"

	"github.com/qobulov/brothers-app/pkg/config"
)

func TestClientRejectsInvalidConfigurationBeforeDialing(t *testing.T) {
	client := New(&config.Config{})
	if err := client.SendOTP(context.Background(), "ali@example.com", "123456"); err == nil {
		t.Fatal("SendOTP() error = nil, want configuration error")
	}
}

func TestClientRequiresSMTPConfiguration(t *testing.T) {
	client := New(&config.Config{
		SMTPHost:     "smtp.example.com",
		SMTPPort:     25,
		SMTPUsername: "user",
		SMTPPassword: "secret",
		SMTPFrom:     "",
	})
	if err := client.SendOTP(context.Background(), "ali@example.com", "123456"); err == nil {
		t.Fatal("SendOTP() error = nil, want configuration error")
	}
}

func TestClientMessageIncludesHTMLAndPlainTextAlternatives(t *testing.T) {
	client := New(&config.Config{SMTPFrom: "noreply@example.com"})
	message, err := client.message("ali@example.com", "482910")
	if err != nil {
		t.Fatalf("message() error = %v", err)
	}

	for _, value := range []string{
		"Content-Type: multipart/alternative",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Type: text/html; charset=UTF-8",
		"Brothers akkauntingiz uchun tasdiqlash kodi: 482910",
		"<html lang=\"uz\">",
		">482910</div>",
	} {
		if !strings.Contains(message, value) {
			t.Errorf("message does not contain %q", value)
		}
	}
}
