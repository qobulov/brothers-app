package telegram

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestClientGetMe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/bottest-token/getMe" {
			t.Fatalf("path = %q, want %q", request.URL.Path, "/bottest-token/getMe")
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"ok":true,"result":{"id":42,"is_bot":true,"username":"brotherzzbot"}}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.URL, 1, 0)
	bot, err := client.GetMe(t.Context())
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	if bot.ID != 42 || !bot.IsBot || bot.Username != "brotherzzbot" {
		t.Fatalf("GetMe() = %+v", bot)
	}
}

func TestClientTransportErrorRedactsToken(t *testing.T) {
	const token = "secret-bot-token"
	client := NewClient(token, "https://api.telegram.org", 1, 0)
	client.httpClient.Transport = roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return nil, errors.New("dialing " + request.URL.String() + ": connection reset")
	})

	err := client.SendMessage(context.Background(), 42, "otp")
	if err == nil {
		t.Fatal("SendMessage() error = nil, want transport error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("SendMessage() error leaked token: %v", err)
	}
	if !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("SendMessage() error = %v, want transport cause", err)
	}
}
