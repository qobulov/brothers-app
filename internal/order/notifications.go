package order

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

const (
	notifyOrderCreated        = "ORDER_CREATED"
	notifyOrderUpdated        = "ORDER_UPDATED"
	notifyOrderAmountMismatch = "ORDER_AMOUNT_MISMATCH"
	notifyOrderCompleted      = "ORDER_COMPLETED"
)

type orderNotification struct {
	eventType  string
	groupID    uuid.UUID
	orderID    uuid.UUID
	amountUSD  int64
	recipients []uuid.UUID
}

type translations struct{ English, Uzbek, Russian string }

func notificationTexts(eventType string, amountUSD int64) (translations, translations) {
	switch eventType {
	case notifyOrderCreated:
		return translations{"New order", "Yangi buyurtma", "Новый заказ"},
			translations{
				fmt.Sprintf("You were assigned to a $%d order", amountUSD),
				fmt.Sprintf("Sizga $%d lik buyurtma biriktirildi", amountUSD),
				fmt.Sprintf("Вам назначен заказ на $%d", amountUSD),
			}
	case notifyOrderUpdated:
		return translations{"Order updated", "Buyurtma o'zgartirildi", "Заказ изменён"},
			translations{
				"An order was changed. Please confirm it again.",
				"Buyurtma o'zgartirildi. Iltimos, qayta tasdiqlang.",
				"Заказ изменён. Пожалуйста, подтвердите его снова.",
			}
	case notifyOrderAmountMismatch:
		return translations{"Amount mismatch", "Summa mos kelmadi", "Суммы не совпадают"},
			translations{
				"Confirmed amounts for an order do not match.",
				"Buyurtma bo'yicha tasdiqlangan summalar mos kelmadi.",
				"Подтверждённые суммы по заказу не совпадают.",
			}
	default:
		return translations{"Order completed", "Buyurtma yakunlandi", "Заказ выполнен"},
			translations{
				fmt.Sprintf("The $%d order is completed.", amountUSD),
				fmt.Sprintf("$%d lik buyurtma yakunlandi.", amountUSD),
				fmt.Sprintf("Заказ на $%d выполнен.", amountUSD),
			}
	}
}

func notify(ctx context.Context, q querier, n orderNotification, now time.Time) error {
	if len(n.recipients) == 0 {
		return nil
	}
	payload, err := json.Marshal(map[string]string{
		"event_type": n.eventType,
		"order_id":   n.orderID.String(),
		"group_id":   n.groupID.String(),
	})
	if err != nil {
		return fmt.Errorf("encoding order notification: %w", err)
	}
	title, content := notificationTexts(n.eventType, n.amountUSD)
	recipients := make([]string, len(n.recipients))
	for i, id := range n.recipients {
		recipients[i] = id.String()
	}
	_, err = q.Exec(ctx, `
		WITH notification AS (
			INSERT INTO notifications (
				title_en, title_uz, title_ru, content_en, content_uz, content_ru,
				type, payload, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, 'TARGETED'::notification_type, $7, $8, $8)
			RETURNING id
		)
		INSERT INTO notification_recipients (notification_id, user_id, is_read, created_at, updated_at)
		SELECT notification.id, recipient::uuid, false, $8, $8
		FROM notification, unnest($9::text[]) AS recipient
	`, title.English, title.Uzbek, title.Russian, content.English, content.Uzbek, content.Russian,
		payload, now, recipients)
	if err != nil {
		return fmt.Errorf("creating %s notification: %w", n.eventType, err)
	}
	return nil
}

// otherParties lists the distinct parties except the actor, who already
// knows what they did. Pass uuid.Nil as the actor to notify everyone.
func otherParties(actorID uuid.UUID, userIDs ...uuid.UUID) []uuid.UUID {
	result := make([]uuid.UUID, 0, len(userIDs))
	for _, id := range userIDs {
		if id != actorID && !slices.Contains(result, id) {
			result = append(result, id)
		}
	}
	return result
}
