// Package email sends authentication messages through an SMTP relay.
package email

import (
	"bytes"
	"context"
	"crypto/tls"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/helpers"
)

const timeout = 10 * time.Second

//go:embed otp.html
var otpTemplateSource string

var otpTemplate = template.Must(template.New("otp.html").Parse(otpTemplateSource))

type Client struct {
	host     string
	port     int
	username string
	password string
	from     mail.Address
}

func New(cfg *config.Config) *Client {
	if cfg == nil {
		return &Client{}
	}
	return &Client{
		host:     strings.TrimSpace(cfg.SMTPHost),
		port:     cfg.SMTPPort,
		username: strings.TrimSpace(cfg.SMTPUsername),
		password: cfg.SMTPPassword,
		from: mail.Address{
			Name:    "Brothers",
			Address: strings.TrimSpace(cfg.SMTPFrom),
		},
	}
}

// SendOTP delivers one short-lived authentication code. Neither the recipient
// nor the code is logged by this package.
func (c *Client) SendOTP(ctx context.Context, recipient, code string) error {
	if err := c.validate(recipient, code); err != nil {
		return err
	}

	connection, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return fmt.Errorf("setting smtp deadline: %w", err)
	}

	client, err := smtp.NewClient(connection, c.host)
	if err != nil {
		return fmt.Errorf("creating smtp client: %w", err)
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return fmt.Errorf("smtp server does not support starttls")
	}
	if err := client.StartTLS(c.tlsConfig()); err != nil {
		return fmt.Errorf("starting smtp tls: %w", err)
	}
	if c.username != "" {
		auth := smtp.PlainAuth("", c.username, c.password, c.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authenticating smtp client: %w", err)
		}
	}
	if err := client.Mail(c.from.Address); err != nil {
		return fmt.Errorf("setting smtp sender: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("setting smtp recipient: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("opening smtp message body: %w", err)
	}
	message, err := c.message(recipient, code)
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(message)); err != nil {
		if closeErr := writer.Close(); closeErr != nil {
			return fmt.Errorf("writing smtp message body: %w", errors.Join(err, closeErr))
		}
		return fmt.Errorf("writing smtp message body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("sending smtp message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("closing smtp client: %w", err)
	}
	return nil
}

func (c *Client) validate(recipient, code string) error {
	if c.host == "" || c.port < 1 || c.port > 65535 || c.from.Address == "" {
		return fmt.Errorf("smtp is not configured")
	}
	if _, err := helpers.NormalizeEmail(c.from.Address); err != nil {
		return fmt.Errorf("invalid smtp sender: %w", err)
	}
	if _, err := helpers.NormalizeEmail(recipient); err != nil {
		return fmt.Errorf("invalid smtp recipient: %w", err)
	}
	if !helpers.ValidOTP(code) {
		return fmt.Errorf("invalid otp")
	}
	return nil
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}
	address := net.JoinHostPort(c.host, strconv.Itoa(c.port))
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("dialing smtp server: %w", err)
	}
	return connection, nil
}

func (c *Client) tlsConfig() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, ServerName: c.host}
}

func (c *Client) message(recipient, code string) (string, error) {
	const boundary = "BrothersOTPBoundary"
	htmlContent, err := renderOTPTemplate(code)
	if err != nil {
		return "", fmt.Errorf("rendering otp email template: %w", err)
	}

	return strings.Join([]string{
		"From: " + c.from.String(),
		"To: " + recipient,
		"Subject: Brothers tasdiqlash kodi",
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=\"" + boundary + "\"",
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		plainTextBody(code),
		"",
		"--" + boundary,
		"Content-Type: text/html; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		htmlContent,
		"",
		"--" + boundary + "--",
		"",
	}, "\r\n"), nil
}

func plainTextBody(code string) string {
	return strings.Join([]string{
		"Salom!",
		"",
		"Brothers akkauntingiz uchun tasdiqlash kodi: " + code,
		"Kod 2 daqiqa davomida amal qiladi.",
		"",
		"Agar bu kodni siz so‘ramagan bo‘lsangiz, ushbu xatni e’tiborsiz qoldiring.",
	}, "\r\n")
}

func renderOTPTemplate(code string) (string, error) {
	var body bytes.Buffer
	if err := otpTemplate.Execute(&body, struct{ OTP string }{OTP: code}); err != nil {
		return "", err
	}
	return body.String(), nil
}
