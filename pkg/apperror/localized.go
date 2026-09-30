package apperror

import (
	"fmt"
	"strconv"
)

// Text is a user-facing message in every supported language.
type Text struct {
	UZ, RU, EN string
}

// Localized is an error kind, such as ErrInvalidData, with a specific reason
// the user should see. The reason becomes the response message in the
// request's language; errors.Is still matches the kind.
type Localized struct {
	kind error
	text Text
}

// New returns kind with a translated, user-facing reason.
func New(kind error, text Text) error {
	return &Localized{kind: kind, text: text}
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
	limit, offset := FormatNumber(int64(maxLimit)), FormatNumber(int64(maxOffset))
	return New(ErrInvalidData, Text{
		UZ: fmt.Sprintf("limit 1 dan %s gacha, offset 0 dan %s gacha bo'lishi kerak", limit.UZ, offset.UZ),
		RU: fmt.Sprintf("limit должен быть от 1 до %s, offset от 0 до %s", limit.RU, offset.RU),
		EN: fmt.Sprintf("limit must be 1-%s and offset 0-%s", limit.EN, offset.EN),
	})
}

func NotAnInteger(field string) error {
	return New(ErrInvalidData, Text{
		UZ: fmt.Sprintf("%s butun son bo'lishi kerak", field),
		RU: fmt.Sprintf("%s должен быть целым числом", field),
		EN: fmt.Sprintf("%s must be an integer", field),
	})
}

func OutOfRange(field string, min, max int) error {
	low, high := FormatNumber(int64(min)), FormatNumber(int64(max))
	return New(ErrInvalidData, Text{
		UZ: fmt.Sprintf("%s %s dan %s gacha bo'lishi kerak", field, low.UZ, high.UZ),
		RU: fmt.Sprintf("%s должен быть от %s до %s", field, low.RU, high.RU),
		EN: fmt.Sprintf("%s must be between %s and %s", field, low.EN, high.EN),
	})
}

func ReasonTooLong(max int) error {
	return New(ErrInvalidData, Text{
		UZ: fmt.Sprintf("Izoh %d belgidan oshmasligi kerak", max),
		RU: fmt.Sprintf("Причина должна быть не длиннее %d символов", max),
		EN: fmt.Sprintf("The reason must be at most %d characters", max),
	})
}

func InvalidEmail() error {
	return New(ErrInvalidData, Text{
		UZ: "Email manzili noto'g'ri",
		RU: "Некорректный email",
		EN: "Enter a valid email address",
	})
}

// FormatNumber groups digits for reading: "1 000 000" in Uzbek and Russian,
// "1,000,000" in English.
func FormatNumber(n int64) Text {
	spaced := groupDigits(n, " ")
	return Text{UZ: spaced, RU: spaced, EN: groupDigits(n, ",")}
}

func groupDigits(n int64, separator string) string {
	digits := strconv.FormatInt(n, 10)
	sign := ""
	if n < 0 {
		sign, digits = "-", digits[1:]
	}
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + separator + digits[i:]
	}
	return sign + digits
}
