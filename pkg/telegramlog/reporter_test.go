package telegramlog

import (
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/responses"
)

func TestReporterDeliversEscapedAndRedactedError(t *testing.T) {
	t.Parallel()
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		requests <- payload
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()
	r := newReporter(server.URL, defaultChatID, server.Client(), 0, []string{"smtp-secret"})
	r.Report(responses.FailureReport{Method: "POST", Path: "/api/v1/auth/otp/send", Environment: "test", Status: 500, Code: 1500,
		Slug: "internal_error", Reason: `ERROR: column <email> missing; password="smtp-secret" otp=123456 Authorization: Bearer abc.def.xyz postgres://user:dbsecret@db/app`,
		Meta: responses.Meta{RequestID: "test-request-123", Timestamp: "2026-09-27T12:00:00Z"}})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
	payload := <-requests
	message := payload["text"].(string)
	if payload["chat_id"] != defaultChatID || payload["parse_mode"] != "HTML" {
		t.Fatalf("invalid destination/format: %v", payload)
	}
	for _, expected := range []string{"test-request-123", "&lt;email&gt;", "POST /api/v1/auth/otp/send", "[REDACTED]"} {
		if !strings.Contains(message, expected) {
			t.Errorf("missing %q", expected)
		}
	}
	for _, secret := range []string{"smtp-secret", "123456", "abc.def.xyz", "dbsecret"} {
		if strings.Contains(message, secret) {
			t.Errorf("secret leaked: %q", secret)
		}
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestReportDoesNotBlockAndShutdownCancelsDelivery(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	client := &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	r := newReporter("https://example.invalid/bot-test/sendMessage", defaultChatID, client, 0, nil)
	r.Report(responses.FailureReport{Reason: "test failure"})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("report did not reach sender")
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() { defer wg.Done(); r.Report(responses.FailureReport{Reason: "another failure"}) }()
	}
	wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Close(ctx); err != context.Canceled {
		t.Fatalf("Close() = %v", err)
	}
	r.Report(responses.FailureReport{Reason: "ignored after close"})
	if err := r.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSendChecksTelegramResult(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		status int
		body   string
		delay  time.Duration
	}{
		{"bot denied", 200, `{"ok":false,"description":"bot cannot send messages"}`, 0},
		{"rate limit", 429, `{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":3}}`, 3 * time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body)), Header: make(http.Header)}, nil
			})}
			r := &Reporter{endpoint: "https://example.invalid", client: client, chatID: defaultChatID}
			delay, err := r.send(context.Background(), "test")
			if err == nil || delay != tt.delay {
				t.Fatalf("send() = %v, %v", delay, err)
			}
		})
	}
}

func TestMessageFitsTelegramLimit(t *testing.T) {
	r := &Reporter{}
	value := strings.Repeat("😀<&>", 3000)
	text := r.format(responses.FailureReport{Reason: value, Method: value, Path: value, Slug: value, Environment: value, Meta: responses.Meta{RequestID: value, Timestamp: value, Duration: value}})
	if count := len(utf16.Encode([]rune(html.UnescapeString(text)))); count > 4096 {
		t.Fatalf("message length = %d", count)
	}
	if !strings.HasSuffix(text, "</pre>") {
		t.Fatal("HTML was truncated mid-tag")
	}
}

func TestMissingTokenDisablesReporting(t *testing.T) {
	r := New(&config.Config{})
	if r != nil {
		t.Fatal("unexpected reporter without token")
	}
	r.Report(responses.FailureReport{Reason: "ignored"})
	if err := r.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
