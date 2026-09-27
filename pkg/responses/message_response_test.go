package responses

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

func TestSuccessUsesAcceptLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		language    string
		wantMessage string
	}{
		{
			name:        "Uzbek",
			language:    "uz-UZ",
			wantMessage: "So'rov muvaffaqiyatli bajarildi",
		},
		{
			name:        "Russian",
			language:    "ru",
			wantMessage: "Запрос успешно обработан",
		},
		{
			name:        "English fallback",
			language:    "de",
			wantMessage: "Request processed successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			Middleware(app, "production")
			app.Get("/", func(c *fiber.Ctx) error {
				return Success[any](c, fiber.StatusOK, nil, "Request processed successfully")
			})

			request := httptest.NewRequest("GET", "/", nil)
			request.Header.Set(fiber.HeaderAcceptLanguage, tt.language)
			response, err := app.Test(request)
			require.NoError(t, err)
			defer response.Body.Close()

			var body Envelope[any]
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			require.Equal(t, tt.wantMessage, body.Message)
		})
	}
}
