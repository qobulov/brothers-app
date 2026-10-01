// Package debt keeps a user's private record of money they owe and money owed
// to them. Debts are not tied to a group and nobody else can see them.
package debt

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/helpers"
)

const (
	DirectionTheyOweMe = "they_owe_me"
	DirectionIOwe      = "i_owe"

	CurrencyUSD = "USD"
	CurrencyUZS = "UZS"

	StatusActive    = "active"
	StatusCompleted = "completed"
	statusAll       = "all"

	maxPersonName = 100
	maxQuery      = 100

	defaultPageSize = 50
	maxPageSize     = 100
	maxOffset       = 10000
)

// Largest amount per currency, in whole units.
var maxAmount = map[string]int64{
	CurrencyUSD: 1_000_000_000,
	CurrencyUZS: 1_000_000_000_000,
}

type Debt struct {
	ID              uuid.UUID  `json:"id"`
	Direction       string     `json:"direction" enums:"they_owe_me,i_owe"`
	PersonName      string     `json:"person_name"`
	PersonPhone     string     `json:"person_phone"`
	Currency        string     `json:"currency" enums:"USD,UZS"`
	OriginalAmount  int64      `json:"original_amount"`
	RemainingAmount int64      `json:"remaining_amount"`
	Status          string     `json:"status" enums:"active,completed"`
	Source          string     `json:"source" enums:"manual,order"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at"`
}

type Repayment struct {
	ID        uuid.UUID `json:"id"`
	Amount    int64     `json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}

// Totals are remaining amounts of active debts in one currency.
type Totals struct {
	TheyOweMe int64 `json:"they_owe_me"`
	IOwe      int64 `json:"i_owe"`
}

// Summary keeps currencies apart; amounts are never converted.
type Summary struct {
	USD Totals `json:"usd"`
	UZS Totals `json:"uzs"`
}

type CreateInput struct {
	Direction   string
	PersonName  string
	PersonPhone string
	Currency    string
	Amount      int64
}

// ListInput with an empty Status lists active debts; Limit 0 uses the default page size.
type ListInput struct {
	Direction string
	Status    string
	Query     string
	Limit     int
	Offset    int
}

// Page with Limit 0 uses the default page size.
type Page struct {
	Limit  int
	Offset int
}

func validCreateInput(input CreateInput) (CreateInput, error) {
	input.Direction = strings.ToLower(strings.TrimSpace(input.Direction))
	if input.Direction != DirectionTheyOweMe && input.Direction != DirectionIOwe {
		return CreateInput{}, invalidDirection()
	}
	input.PersonName = strings.TrimSpace(input.PersonName)
	if input.PersonName == "" || utf8.RuneCountInString(input.PersonName) > maxPersonName {
		return CreateInput{}, apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: "Ism 1 dan 100 belgigacha bo'lishi kerak",
			RU: "Имя должно содержать от 1 до 100 символов",
			EN: "The name must be 1-100 characters",
		})
	}
	if phone := strings.TrimSpace(input.PersonPhone); phone != "" {
		normalized, err := helpers.NormalizePhone(phone)
		if err != nil {
			return CreateInput{}, apperror.New(apperror.ErrInvalidData, apperror.Text{
				UZ: "Telefon raqami noto'g'ri", RU: "Некорректный номер телефона", EN: "The phone number is invalid",
			})
		}
		input.PersonPhone = normalized
	} else {
		input.PersonPhone = ""
	}
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	max, ok := maxAmount[input.Currency]
	if !ok {
		return CreateInput{}, apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: "Valyuta USD yoki UZS bo'lishi kerak", RU: "Валюта должна быть USD или UZS", EN: "Currency must be USD or UZS",
		})
	}
	if input.Amount < 1 || input.Amount > max {
		limit := apperror.FormatNumber(max)
		return CreateInput{}, apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: fmt.Sprintf("Summa 1 dan %s %s gacha bo'lishi kerak", limit.UZ, input.Currency),
			RU: fmt.Sprintf("Сумма должна быть от 1 до %s %s", limit.RU, input.Currency),
			EN: fmt.Sprintf("The amount must be between 1 and %s %s", limit.EN, input.Currency),
		})
	}
	return input, nil
}

func validListInput(input ListInput) (ListInput, error) {
	input.Direction = strings.ToLower(strings.TrimSpace(input.Direction))
	if input.Direction != "" && input.Direction != DirectionTheyOweMe && input.Direction != DirectionIOwe {
		return ListInput{}, invalidDirection()
	}
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	switch input.Status {
	case "":
		input.Status = StatusActive
	case StatusActive, StatusCompleted, statusAll:
	default:
		return ListInput{}, apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: "Holat active, completed yoki all bo'lishi kerak",
			RU: "Статус должен быть active, completed или all",
			EN: "Status must be active, completed or all",
		})
	}
	input.Query = strings.TrimSpace(input.Query)
	if utf8.RuneCountInString(input.Query) > maxQuery {
		return ListInput{}, apperror.New(apperror.ErrInvalidData, apperror.Text{
			UZ: "Qidiruv matni 100 belgidan oshmasligi kerak",
			RU: "Поисковый запрос не должен превышать 100 символов",
			EN: "The search query must be at most 100 characters",
		})
	}
	page, err := validPage(Page{Limit: input.Limit, Offset: input.Offset})
	if err != nil {
		return ListInput{}, err
	}
	input.Limit, input.Offset = page.Limit, page.Offset
	return input, nil
}

func validPage(page Page) (Page, error) {
	if page.Limit == 0 {
		page.Limit = defaultPageSize
	}
	if page.Limit < 1 || page.Limit > maxPageSize || page.Offset < 0 || page.Offset > maxOffset {
		return Page{}, apperror.PageOutOfRange(maxPageSize, maxOffset)
	}
	return page, nil
}

// phoneDigits keeps only digits, so "90 777-44" finds "+998907774422".
func phoneDigits(query string) string {
	var digits strings.Builder
	for _, r := range query {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	return digits.String()
}

func invalidDirection() error {
	return apperror.New(apperror.ErrInvalidData, apperror.Text{
		UZ: "Yo'nalish they_owe_me yoki i_owe bo'lishi kerak",
		RU: "Направление должно быть they_owe_me или i_owe",
		EN: "Direction must be they_owe_me or i_owe",
	})
}

func notFound() error {
	return apperror.New(apperror.ErrRecordNotFound, apperror.Text{
		UZ: "Qarz topilmadi", RU: "Долг не найден", EN: "Debt not found",
	})
}

func alreadyCompleted() error {
	return apperror.New(apperror.ErrConflict, apperror.Text{
		UZ: "Qarz allaqachon yopilgan", RU: "Долг уже закрыт", EN: "The debt is already completed",
	})
}
