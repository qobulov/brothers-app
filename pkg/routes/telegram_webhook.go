package routes

import (
	"crypto/subtle"
	"encoding/json"
	"log"
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
			return c.SendStatus(fiber.StatusBadRequest)
		}
		if err := telegram.HandleUpdate(c.UserContext(), auth, update); err != nil {
			// Acknowledge rejected updates so Telegram does not retry a consumed
			// or invalid deep-link token indefinitely.
			log.Printf("telegram webhook update %d rejected: %v", update.UpdateID, err)
		}
		return c.SendStatus(fiber.StatusOK)
	}
}
