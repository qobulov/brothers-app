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
	r := newReporter(server.URL, defaultChatID, 0, server.Client(), 0, []string{"smtp-secret"})
	r.Report(responses.FailureReport{Method: "POST", Path: "/api/v1/auth/otp/send", Environment: "test", Status: 500, Code: 1500,
		Slug: "internal_error", Reason: `ERROR: column <email> missing; password="smtp-secret" otp=123456 Authorization: Bearer abc.def.xyz postgres://user:dbsecret@db/app`,
		RequestBody:  `{"email":"ali@example.com","username":"qobulov","purpose":"registration","password":"body-secret"}`,
		ResponseBody: `{"success":false,"code":1500,"data":{"email":"ali@example.com"}}`,
		Meta:         responses.Meta{RequestID: "test-request-123", Timestamp: "2026-09-27T12:00:00Z"}})
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
	if _, ok := payload["message_thread_id"]; ok {
		t.Fatalf("unexpected General topic thread ID: %v", payload)
	}
	for _, expected := range []string{"test-request-123", "&lt;email&gt;", "POST /api/v1/auth/otp/send", "Request body", "Response body", "registration", "[REDACTED]", "\n  &#34;purpose&#34;"} {
		if !strings.Contains(message, expected) {
			t.Errorf("missing %q", expected)
		}
	}
	for _, secret := range []string{"smtp-secret", "123456", "abc.def.xyz", "dbsecret", "ali@example.com", "qobulov", "body-secret"} {
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
	r := newReporter("https://example.invalid/bot-test/sendMessage", defaultChatID, 0, client, 0, nil)
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

func TestReporterDeliversToConfiguredForumTopic(t *testing.T) {
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

	r := newReporter(server.URL, defaultChatID, 123, server.Client(), 0, nil)
	r.Report(responses.FailureReport{Reason: "test failure"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if got := (<-requests)["message_thread_id"]; got != float64(123) {
		t.Fatalf("message_thread_id = %v, want 123", got)
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
	text := r.format(responses.FailureReport{Reason: value, RequestBody: value, ResponseBody: value, Method: value, Path: value, Slug: value, Environment: value, Meta: responses.Meta{RequestID: value, Timestamp: value, Duration: value}})
	if count := len(utf16.Encode([]rune(html.UnescapeString(text)))); count > 4096 {
		t.Fatalf("message length = %d", count)
	}
	if !strings.HasSuffix(text, "</pre>") {
		t.Fatal("HTML was truncated mid-tag")
	}
}

func TestDevelopmentReportShowsFullRequestBody(t *testing.T) {
	t.Parallel()
	r := &Reporter{secrets: []string{"body-secret"}}
	text := html.UnescapeString(r.format(responses.FailureReport{
		Environment:  "development",
		RequestBody:  `{"email":"ali@example.com","username":"qobulov","password":"body-secret"}`,
		ResponseBody: `{"data":{"email":"ali@example.com"}}`,
	}))
	for _, value := range []string{"ali@example.com", "qobulov", "body-secret"} {
		if !strings.Contains(text, value) {
			t.Fatalf("development request body is missing %q: %s", value, text)
		}
	}
}

func TestPrettyJSON(t *testing.T) {
	t.Parallel()
	got := prettyJSON(`{"email":"ali@example.com","nested":{"purpose":"registration"}}`)
	want := "{\n  \"email\": \"ali@example.com\",\n  \"nested\": {\n    \"purpose\": \"registration\"\n  }\n}"
	if got != want {
		t.Fatalf("prettyJSON() = %q, want %q", got, want)
	}
}

func TestPrettyRedactedJSONRemainsValid(t *testing.T) {
	t.Parallel()
	r := &Reporter{}
	got := r.prettyRedactedJSON(`{"email":"ali@example.com","purpose":"registration","nested":{"refresh_token":"secret"}}`)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("redacted body is not valid JSON: %v\n%s", err, got)
	}
	if decoded["email"] != "[REDACTED]" || decoded["purpose"] != "registration" {
		t.Fatalf("unexpected redacted JSON: %v", decoded)
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
