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
			name:        "Accept-Language priority",
			language:    "uz;q=0.1,ru;q=0.9,en;q=0.8",
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
				return Success[any](c, fiber.StatusOK, nil, MessageRequestProcessed)
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

func TestLocalizeMessageCoversHandlerMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		message string
		uz      string
		ru      string
		en      string
	}{
		{MessageRequestProcessed, "So'rov muvaffaqiyatli bajarildi", "Запрос успешно обработан", "Request processed successfully"},
		{MessageGroupCreated, "Guruh yaratildi", "Группа создана", "Group created"},
		{MessageGroupsReturned, "Guruhlar olindi", "Группы получены", "Groups returned"},
		{MessageGroupDeleted, "Guruh o'chirildi", "Группа удалена", "Group deleted"},
		{MessageInvitationCreated, "Taklif yaratildi", "Приглашение создано", "Invitation created"},
		{MessageMembersReturned, "A'zolar olindi", "Участники получены", "Members returned"},
		{MessageInvitationAction, "Taklif javobi qayta ishlandi", "Ответ на приглашение обработан", "Invitation action processed"},
		{MessageUsersReturned, "Foydalanuvchilar olindi", "Пользователи получены", "Users returned"},
		{MessageNotificationsReturned, "Bildirishnomalar olindi", "Уведомления получены", "Notifications returned"},
		{MessageUserDeleted, "Foydalanuvchi o'chirildi", "Пользователь удалён", "User deleted"},
		{MessageOrderCreated, "Buyurtma yaratildi", "Заказ создан", "Order created"},
		{MessageOrdersReturned, "Buyurtmalar olindi", "Заказы получены", "Orders returned"},
		{MessageOrderUpdated, "Buyurtma o'zgartirildi", "Заказ изменён", "Order updated"},
		{MessageOrderConfirmed, "Buyurtma tasdiqlandi", "Заказ подтверждён", "Order confirmed"},
		{MessageResourceNotFound, "Resurs topilmadi", "Ресурс не найден", "Resource not found"},
		{MessageInvalidCredentials, "Hisob ma'lumotlari noto'g'ri", "Неверные учетные данные", "Invalid credentials"},
		{MessageInvalidRequest, "So'rov ma'lumotlari noto'g'ri", "Некорректный запрос", "Invalid request"},
		{MessageInvalidID, "ID noto'g'ri", "Некорректный ID", "Invalid ID"},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			require.Equal(t, tt.uz, localizeMessage(tt.message, "uz-UZ"))
			require.Equal(t, tt.ru, localizeMessage(tt.message, "ru-RU"))
			require.Equal(t, tt.en, localizeMessage(tt.message, "de-DE"))
		})
	}
}
