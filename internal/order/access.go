package order

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/database"
	"github.com/qobulov/brothers-app/pkg/helpers"
)

// displayName is the SQL expression for a person's name, matching the group package.
func displayName(alias string) string {
	return fmt.Sprintf(`COALESCE(NULLIF(btrim(concat_ws(' ', %[1]s.first_name, %[1]s.last_name)), ''), NULLIF(%[1]s.name, ''), NULLIF(%[1]s.username, ''), '')`, alias)
}

func loadViewer(ctx context.Context, q database.Querier, groupID, userID uuid.UUID) (viewer, error) {
	v := viewer{userID: userID}
	err := q.QueryRow(ctx, `
		SELECT members.id, members.role::text, members.is_owner
		FROM group_members members
		JOIN groups ON groups.id = members.group_id
		WHERE members.group_id = $1
		  AND members.user_id = $2
		  AND members.deleted_at IS NULL
		  AND groups.deleted_at IS NULL
		  AND groups.is_active
	`, groupID, userID).Scan(&v.memberID, &v.role, &v.isOwner)
	if errors.Is(err, pgx.ErrNoRows) {
		return viewer{}, apperror.ErrRecordNotFound
	}
	if err != nil {
		return viewer{}, fmt.Errorf("loading group membership: %w", err)
	}
	return v, nil
}

// canWrite is the create/edit rule: managers act for any two employees,
// employees only for orders they take part in, investors never.
func (v viewer) canWrite(giverUserID, receiverUserID uuid.UUID) error {
	if v.isManager() {
		return nil
	}
	if v.isEmployee() && (v.userID == giverUserID || v.userID == receiverUserID) {
		return nil
	}
	return apperror.ErrForbidden
}

type partyRef struct {
	memberID   uuid.UUID
	locationID *uuid.UUID
}

// resolveEmployee finds an active employee membership and its current location.
func resolveEmployee(ctx context.Context, q database.Querier, groupID, userID uuid.UUID) (partyRef, error) {
	var ref partyRef
	err := q.QueryRow(ctx, `
		SELECT members.id, locations.id
		FROM group_members members
		JOIN users ON users.id = members.user_id AND users.deleted_at IS NULL AND users.is_active
		LEFT JOIN locations
		  ON locations.group_id = members.group_id
		 AND locations.employee_id = members.id
		 AND locations.deleted_at IS NULL
		WHERE members.group_id = $1
		  AND members.user_id = $2
		  AND members.role::text = 'employee'
		  AND members.deleted_at IS NULL
	`, groupID, userID).Scan(&ref.memberID, &ref.locationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return partyRef{}, apperror.New(apperror.ErrRecordNotFound, apperror.Text{UZ: "Buyurtma ishtirokchisi guruhning faol xodimi bo'lishi kerak", RU: "Участник заказа должен быть активным сотрудником группы", EN: "Order parties must be active employees of the group"})
	}
	if err != nil {
		return partyRef{}, fmt.Errorf("resolving order party: %w", err)
	}
	return ref, nil
}

// upsertCustomer finds or creates the group's customer for an already normalized phone.
func upsertCustomer(ctx context.Context, q database.Querier, groupID uuid.UUID, phone string, now time.Time) (uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, `
		INSERT INTO customers (group_id, phone, created_at, updated_at)
		VALUES ($1, $2, $3, $3)
		ON CONFLICT (group_id, phone) WHERE deleted_at IS NULL
		DO UPDATE SET updated_at = EXCLUDED.updated_at
		RETURNING id
	`, groupID, phone, now).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("saving order customer: %w", err)
	}
	return id, nil
}

func validCreateInput(input CreateInput) (CreateInput, error) {
	if input.GiverUserID == uuid.Nil || input.ReceiverUserID == uuid.Nil || input.GiverUserID == input.ReceiverUserID {
		return CreateInput{}, apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Beruvchi va oluvchi turli xodimlar bo'lishi kerak", RU: "Отправитель и получатель должны быть разными сотрудниками", EN: "The giver and receiver must be two different employees"})
	}
	if err := validAmounts(input.AmountUSD, input.FeeUZS); err != nil {
		return CreateInput{}, err
	}
	var err error
	if input.GiverCustomerPhone, err = validPhone(input.GiverCustomerPhone); err != nil {
		return CreateInput{}, err
	}
	if input.ReceiverCustomerPhone, err = validPhone(input.ReceiverCustomerPhone); err != nil {
		return CreateInput{}, err
	}
	return input, nil
}

func validAmounts(amountUSD, feeUZS int64) error {
	if amountUSD < 1 || amountUSD > maxAmountUSD {
		max := apperror.FormatNumber(maxAmountUSD)
		return apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: fmt.Sprintf("Summa 1 dan %s dollargacha bo'lishi kerak", max.UZ),
			RU: fmt.Sprintf("Сумма должна быть от 1 до %s долларов", max.RU),
			EN: fmt.Sprintf("The amount must be between $1 and $%s", max.EN),
		})
	}
	if feeUZS < 0 || feeUZS > maxFeeUZS {
		max := apperror.FormatNumber(maxFeeUZS)
		return apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: fmt.Sprintf("Xizmat haqi 0 dan %s so'mgacha bo'lishi kerak", max.UZ),
			RU: fmt.Sprintf("Комиссия должна быть от 0 до %s сумов", max.RU),
			EN: fmt.Sprintf("The fee must be between 0 and %s UZS", max.EN),
		})
	}
	return nil
}

func validPhone(value string) (string, error) {
	phone, err := helpers.NormalizePhone(value)
	if err != nil {
		return "", apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Mijoz telefon raqami noto'g'ri", RU: "Некорректный номер телефона клиента", EN: "The customer phone number is invalid"})
	}
	return phone, nil
}
