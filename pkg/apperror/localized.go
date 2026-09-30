package apperror

import (
	"errors"
	"fmt"
)

// Text is a user-facing message in every supported language.
type Text struct {
	UZ, RU, EN string
}

// Localized is an error kind, such as ErrInvalidData, with a specific reason
// the user should see. The reason becomes the response message in the
// request's language; errors.Is still matches the kind.
type Localized struct {
	kind  error
	field string
	text  Text
}

// New returns kind with a translated, user-facing reason.
func New(kind error, text Text) error {
	return &Localized{kind: kind, text: text}
}

// NewField is New for an error caused by one request field, so the app can
// show the message next to that input.
func NewField(kind error, field string, text Text) error {
	return &Localized{kind: kind, field: field, text: text}
}

// Field returns the request field an error is about, or "" when there is none.
func Field(err error) string {
	var specific *Localized
	if errors.As(err, &specific) {
		return specific.field
	}
	return ""
}

func (e *Localized) Error() string { return e.kind.Error() + ": " + e.text.EN }

func (e *Localized) Unwrap() error { return e.kind }

// Message returns the reason in the resolved language (uz, ru or en).
func (e *Localized) Message(language string) string {
	return localized(language, e.text.UZ, e.text.RU, e.text.EN)
}

// Messages shared by several packages.

func NothingToUpdate() error {
	return New(ErrInvalidData, Text{
		UZ: "O'zgartirish uchun hech narsa yuborilmadi",
		RU: "Нет данных для изменения",
		EN: "Nothing to update",
	})
}

func PageOutOfRange(maxLimit, maxOffset int) error {
	return New(ErrInvalidData, Text{
		UZ: fmt.Sprintf("limit 1 dan %d gacha, offset 0 dan %d gacha bo'lishi kerak", maxLimit, maxOffset),
		RU: fmt.Sprintf("limit должен быть от 1 до %d, offset от 0 до %d", maxLimit, maxOffset),
		EN: fmt.Sprintf("limit must be 1-%d and offset 0-%d", maxLimit, maxOffset),
	})
}

func NotAnInteger(field string) error {
	return NewField(ErrInvalidData, field, Text{
		UZ: fmt.Sprintf("%s butun son bo'lishi kerak", field),
		RU: fmt.Sprintf("%s должен быть целым числом", field),
		EN: fmt.Sprintf("%s must be an integer", field),
	})
}

func OutOfRange(field string, min, max int) error {
	return NewField(ErrInvalidData, field, Text{
		UZ: fmt.Sprintf("%s %d dan %d gacha bo'lishi kerak", field, min, max),
		RU: fmt.Sprintf("%s должен быть от %d до %d", field, min, max),
		EN: fmt.Sprintf("%s must be between %d and %d", field, min, max),
	})
}

func ReasonTooLong(max int) error {
	return NewField(ErrInvalidData, "reason", Text{
		UZ: fmt.Sprintf("Izoh %d belgidan oshmasligi kerak", max),
		RU: fmt.Sprintf("Причина должна быть не длиннее %d символов", max),
		EN: fmt.Sprintf("The reason must be at most %d characters", max),
	})
}

func InvalidEmail() error {
	return NewField(ErrInvalidData, "email", Text{
		UZ: "Email manzili noto'g'ri",
		RU: "Некорректный email",
		EN: "Enter a valid email address",
	})
}
