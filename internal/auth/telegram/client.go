package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
	poll       time.Duration
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

type Message struct {
	Chat Chat   `json:"chat"`
	Text string `json:"text"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type Bot struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username"`
}

func NewClient(token, baseURL string, requestTimeout, pollTimeout int) *Client {
	if requestTimeout < pollTimeout+5 {
		requestTimeout = pollTimeout + 5
	}
	return &Client{token: token, baseURL: strings.TrimRight(baseURL, "/"), httpClient: &http.Client{Timeout: time.Duration(requestTimeout) * time.Second}, poll: time.Duration(pollTimeout) * time.Second}
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	var ignored struct{}
	return c.call(ctx, "sendMessage", url.Values{"chat_id": {strconv.FormatInt(chatID, 10)}, "text": {text}}, &ignored)
}

func (c *Client) GetMe(ctx context.Context) (Bot, error) {
	var bot Bot
	if err := c.call(ctx, "getMe", nil, &bot); err != nil {
		return Bot{}, err
	}
	return bot, nil
}

func (c *Client) GetUpdates(ctx context.Context, offset int64) ([]Update, error) {
	var updates []Update
	values := url.Values{"timeout": {strconv.Itoa(int(c.poll.Seconds()))}}
	if offset > 0 {
		values.Set("offset", strconv.FormatInt(offset, 10))
	}
	if err := c.call(ctx, "getUpdates", values, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *Client) call(ctx context.Context, method string, values url.Values, out interface{}) error {
	endpoint := fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return fmt.Errorf("creating telegram request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.httpClient.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("calling telegram api method %s: %w", method, ctxErr)
		}
		return fmt.Errorf("calling telegram api method %s: %s", method, redactCredential(err.Error(), c.token))
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("telegram api returned status %d", response.StatusCode)
	}
	var envelope struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decoding telegram response: %w", err)
	}
	if !envelope.OK {
		return fmt.Errorf("telegram api rejected request: %s", envelope.Description)
	}
	if out != nil && len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("decoding telegram result: %w", err)
		}
	}
	return nil
}

func redactCredential(message, credential string) string {
	if credential == "" {
		return message
	}
	return strings.ReplaceAll(message, credential, "[redacted]")
}
