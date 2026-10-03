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
	threadID         int
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
	return newReporter("https://api.telegram.org/bot"+token+"/sendMessage", chatID, cfg.TelegramBackendErrorThreadID,
		&http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		3*time.Second, []string{token, cfg.DBPassword, cfg.SMTPPassword, cfg.JWTSecret, cfg.OTPPepper, cfg.DatabaseDSN, cfg.RedisURL})
}

func newReporter(endpoint, chatID string, threadID int, client *http.Client, interval time.Duration, secrets []string) *Reporter {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Reporter{client: client, endpoint: endpoint, chatID: chatID, threadID: threadID, secrets: secrets,
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
	payload := struct {
		ChatID                string `json:"chat_id"`
		MessageThreadID       int    `json:"message_thread_id,omitempty"`
		Text                  string `json:"text"`
		ParseMode             string `json:"parse_mode"`
		DisableWebPagePreview bool   `json:"disable_web_page_preview"`
	}{
		ChatID:                r.chatID,
		MessageThreadID:       r.threadID,
		Text:                  message,
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("encoding telegram report: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
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

var credentialPattern = regexp.MustCompile(`(?i)("?(?:password|passwd|pwd|otp|otp_code|token|access_token|refresh_token|reset_token|secret|authorization|api_key|apikey)"?\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`)

// personalPattern needs a quoted key or "=", so an operation label in an error
// such as "checking username: ..." is not mistaken for a value.
var personalPattern = regexp.MustCompile(`(?i)((?:"(?:email|phone|username|first_name|last_name|avatar_url)"\s*:|\b(?:email|phone|username|first_name|last_name|avatar_url)\s*=)\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
var sensitiveJSONKeyPattern = regexp.MustCompile(`(?i)(password|passwd|pwd|otp|token|secret|authorization|api_?key|email|phone|username|first_name|last_name|avatar_url)`)
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
	value = personalPattern.ReplaceAllString(value, "${1}[REDACTED]")
	return urlPasswordPattern.ReplaceAllString(value, "${1}[REDACTED]@")
}

func (r *Reporter) format(event responses.FailureReport) string {
	plainField := func(value string, limit int) string {
		runes := []rune(value)
		if len(runes) > limit {
			value = string(runes[:limit]) + "… [truncated]"
		}
		return html.EscapeString(value)
	}
	field := func(value string, limit int) string {
		return plainField(r.redact(value), limit)
	}
	bodySection := func(title, body string, limit int) string {
		if body == "" {
			return ""
		}
		if strings.ToLower(strings.TrimSpace(event.Environment)) == "development" {
			body = prettyJSON(body)
		} else {
			body = r.prettyRedactedJSON(body)
		}
		return fmt.Sprintf("\n\n<b>%s</b>\n<pre>%s</pre>", title, plainField(body, limit))
	}
	// Bound every variable section so the rendered text stays below Telegram's
	// 4096 UTF-16 code unit limit, including non-BMP Unicode.
	query := bodySection("Query", event.Query, 400)
	requestBody := bodySection("Request body", event.RequestBody, 800)
	responseBody := ""
	if !onlyRepeatsReason(event.ResponseBody) {
		responseBody = bodySection("Response data", event.ResponseBody, 800)
	}
	return fmt.Sprintf("🔴 <b>%d %s</b> · <code>%s %s</code>\n%s · %s · %s\nRequest ID: <code>%s</code>\n\n<b>Reason</b>\n<pre>%s</pre>%s",
		event.Status, field(event.Slug, 80), field(event.Method, 10), field(event.Path, 200),
		field(tashkentTime(event.Meta.Timestamp), 40), field(shortDuration(event.Meta.Duration), 32), field(event.Environment, 32),
		field(event.Meta.RequestID, 128), field(readableReason(event.Reason), 700), query+requestBody+responseBody)
}

// readableReason shortens pgx connection failures, which list every database
// host on its own line, to the operation and the distinct server messages.
func readableReason(reason string) string {
	const marker = "failed to connect to `"
	start := strings.Index(reason, marker)
	if start < 0 {
		return reason
	}
	_, details, _ := strings.Cut(reason[start+len(marker):], "`:")
	summary := reason[:start] + "cannot connect to the database"
	seen := map[string]bool{}
	for _, line := range strings.Split(details, "\n") {
		message := hostErrorMessage(line)
		if message == "" || seen[message] {
			continue
		}
		seen[message] = true
		summary += "\n• " + message
	}
	return summary
}

// hostErrorMessage drops the "ip:port (host): server error:" prefix of one line.
func hostErrorMessage(line string) string {
	if _, after, found := strings.Cut(line, "server error:"); found {
		line = after
	} else if _, after, found := strings.Cut(line, "): "); found {
		line = after
	}
	line = strings.TrimSuffix(strings.TrimSpace(line), "(SQLSTATE )")
	return strings.TrimSpace(strings.TrimLeft(line, ": "))
}

// onlyRepeatsReason reports a response that carries nothing but the reason
// already shown in the report.
func onlyRepeatsReason(responseBody string) bool {
	var data map[string]any
	if err := json.Unmarshal([]byte(responseBody), &data); err != nil {
		return false
	}
	_, hasReason := data["reason"]
	return hasReason && len(data) == 1
}

var tashkent = time.FixedZone("Toshkent", 5*60*60)

func tashkentTime(timestamp string) string {
	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return timestamp
	}
	return parsed.In(tashkent).Format("2006-01-02 15:04:05") + " Toshkent"
}

func shortDuration(duration string) string {
	parsed, err := time.ParseDuration(duration)
	if err != nil {
		return duration
	}
	switch {
	case parsed >= time.Second:
		return parsed.Round(10 * time.Millisecond).String()
	case parsed >= time.Millisecond:
		return parsed.Round(time.Millisecond).String()
	}
	return duration
}

func prettyJSON(value string) string {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, []byte(value), "", "  "); err != nil {
		return value
	}
	return formatted.String()
}

func (r *Reporter) prettyRedactedJSON(value string) string {
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return r.redact(value)
	}
	decoded = r.redactJSONValue(decoded)
	formatted, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return r.redact(value)
	}
	return string(formatted)
}

func (r *Reporter) redactJSONValue(value any) any {
	switch current := value.(type) {
	case map[string]any:
		for key, item := range current {
			if sensitiveJSONKeyPattern.MatchString(key) {
				current[key] = "[REDACTED]"
				continue
			}
			current[key] = r.redactJSONValue(item)
		}
		return current
	case []any:
		for index, item := range current {
			current[index] = r.redactJSONValue(item)
		}
		return current
	case string:
		return r.redact(current)
	default:
		return current
	}
}
