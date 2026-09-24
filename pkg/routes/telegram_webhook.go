package routes

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/qobulov/brothers-app/internal/auth/telegram"
)

const telegramWebhookSecretHeader = "X-Telegram-Bot-Api-Secret-Token"

func telegramWebhookHandler(secret string, auth telegram.StartHandler) fiber.Handler {
	secret = strings.TrimSpace(secret)
	return func(c *fiber.Ctx) error {
		if secret == "" {
			return c.SendStatus(fiber.StatusNotFound)
		}
		provided := c.Get(telegramWebhookSecretHeader)
		if subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
			return c.SendStatus(fiber.StatusUnauthorized)
		}

		var update telegram.Update
		if err := json.Unmarshal(c.Body(), &update); err != nil {
			slog.WarnContext(c.UserContext(), "telegram webhook rejected", "reason", "invalid_json", "error", err)
			return c.SendStatus(fiber.StatusBadRequest)
		}
		slog.InfoContext(c.UserContext(), "telegram webhook received", "update_id", update.UpdateID)
		if err := telegram.HandleUpdate(c.UserContext(), auth, update); err != nil {
			// Acknowledge rejected updates so Telegram does not retry a consumed
			// or invalid deep-link token indefinitely.
			slog.ErrorContext(c.UserContext(), "telegram webhook update rejected", "update_id", update.UpdateID, "error", err)
		} else {
			slog.InfoContext(c.UserContext(), "telegram webhook update handled", "update_id", update.UpdateID)
		}
		return c.SendStatus(fiber.StatusOK)
	}
}
