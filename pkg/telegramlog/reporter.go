// Package telegramlog delivers bounded, asynchronous API error reports to Telegram.
package telegramlog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/responses"
)

const defaultChatID = "-1003866068293"

type Reporter struct {
	client           *http.Client
	endpoint, chatID string
	secrets          []string
	queue            chan string
	done             chan struct{}
	cancel           context.CancelFunc
	mu               sync.RWMutex
	closed           bool
	interval         time.Duration
}

func New(cfg *config.Config) *Reporter {
	if cfg == nil || strings.TrimSpace(cfg.TelegramBotToken) == "" {
		return nil
	}
	chatID := strings.TrimSpace(cfg.TelegramChatID)
	if chatID == "" {
		chatID = defaultChatID
	}
	token := strings.TrimSpace(cfg.TelegramBotToken)
	return newReporter("https://api.telegram.org/bot"+token+"/sendMessage", chatID,
		&http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		3*time.Second, []string{token, cfg.DBPassword, cfg.SMTPPassword, cfg.JWTSecret, cfg.OTPPepper, cfg.DatabaseDSN, cfg.RedisURL})
}

func newReporter(endpoint, chatID string, client *http.Client, interval time.Duration, secrets []string) *Reporter {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Reporter{client: client, endpoint: endpoint, chatID: chatID, secrets: secrets,
		queue: make(chan string, 64), done: make(chan struct{}), cancel: cancel, interval: interval}
	go r.run(ctx)
	return r
}

// Report formats the snapshot before returning; it never retains a Fiber context.
func (r *Reporter) Report(event responses.FailureReport) {
	if r == nil {
		return
	}
	message := r.format(event)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return
	}
	select {
	case r.queue <- message:
	default:
		slog.Warn("telegram error queue full", "request_id", event.Meta.RequestID)
	}
}

// Close drains pending reports until the deadline, then cancels the worker.
func (r *Reporter) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.queue)
	}
	r.mu.Unlock()
	select {
	case <-r.done:
		r.cancel()
		return nil
	case <-ctx.Done():
		r.cancel()
		<-r.done
		return ctx.Err()
	}
}

func (r *Reporter) run(ctx context.Context) {
	defer close(r.done)
	var next time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-r.queue:
			if !ok {
				return
			}
			if !wait(ctx, time.Until(next)) {
				return
			}
			retryAfter, err := r.send(ctx, message)
			if retryAfter > 0 && wait(ctx, retryAfter) {
				_, err = r.send(ctx, message)
			}
			if err != nil {
				slog.Error("telegram error report delivery failed", "error", r.redact(err.Error()))
			}
			next = time.Now().Add(r.interval)
		}
	}
}

func wait(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (r *Reporter) send(ctx context.Context, message string) (time.Duration, error) {
	payload, err := json.Marshal(map[string]any{"chat_id": r.chatID, "text": message, "parse_mode": "HTML", "disable_web_page_preview": true})
	if err != nil {
		return 0, fmt.Errorf("encoding telegram report: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, errors.New("invalid telegram endpoint")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		// net/http errors include the request URL, which contains the bot token.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return 0, fmt.Errorf("sending telegram report: %w", err)
	}
	defer response.Body.Close()
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16*1024)).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding telegram response (HTTP %d): %w", response.StatusCode, err)
	}
	if response.StatusCode != http.StatusOK || !result.OK {
		delay := time.Duration(0)
		if response.StatusCode == 429 && result.Parameters.RetryAfter > 0 {
			delay = time.Duration(min(result.Parameters.RetryAfter, 60)) * time.Second
		}
		return delay, fmt.Errorf("telegram HTTP %d: %s", response.StatusCode, result.Description)
	}
	return 0, nil
}

var credentialPattern = regexp.MustCompile(`(?i)("?(?:password|passwd|pwd|otp|otp_code|token|access_token|refresh_token|reset_token|secret|authorization|api_key)"?\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
var bearerPattern = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/=-]+`)
var urlPasswordPattern = regexp.MustCompile(`(://[^\s/:@]+:)[^\s@]+@`)

func (r *Reporter) redact(value string) string {
	for _, secret := range r.secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	value = bearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = credentialPattern.ReplaceAllString(value, "${1}[REDACTED]")
	return urlPasswordPattern.ReplaceAllString(value, "${1}[REDACTED]@")
}

func (r *Reporter) format(event responses.FailureReport) string {
	field := func(value string, limit int) string {
		value = r.redact(value)
		runes := []rune(value)
		if len(runes) > limit {
			value = string(runes[:limit]) + "… [truncated]"
		}
		return html.EscapeString(value)
	}
	// Bound the entire rendered text, even when errors contain non-BMP Unicode.
	return fmt.Sprintf("<b>Brothers API error</b>\nEnvironment: <code>%s</code>\nTime (UTC): <code>%s</code>\nEndpoint: <code>%s %s</code>\nHTTP: <code>%d</code> · Code: <code>%d</code>\nSlug: <code>%s</code>\nRequest ID: <code>%s</code>\nDuration: <code>%s</code>\n\n<b>Reason</b>\n<pre>%s</pre>",
		field(event.Environment, 32), field(event.Meta.Timestamp, 40), field(event.Method, 10), field(event.Path, 200),
		event.Status, event.Code, field(event.Slug, 80), field(event.Meta.RequestID, 128), field(event.Meta.Duration, 32), field(event.Reason, 1300))
}
