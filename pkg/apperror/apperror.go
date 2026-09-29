package apperror

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/qobulov/brothers-app/pkg/localization"
)

type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

// Normalize classifies driver/framework errors while retaining the original cause.
func Normalize(err error) error {
	if err == nil {
		return ErrInternalServer
	}
	var kind error
	var pgErr *pgconn.PgError
	var httpErr *fiber.Error
	switch {
	case errors.Is(err, ErrEmailUnavailable):
		return err
	case errors.Is(err, pgx.ErrNoRows):
		kind = ErrRecordNotFound
	case errors.Is(err, context.DeadlineExceeded):
		kind = ErrTimeout
	case errors.As(err, &pgErr):
		switch pgErr.Code {
		case "23505":
			kind = ErrDuplicatedKey
		case "23503":
			kind = ErrForeignKeyViolated
		case "23514":
			kind = ErrCheckConstraintViolated
		case "23502", "22P02", "22003", "22007", "22008":
			kind = ErrInvalidData
		}
	case errors.As(err, &httpErr):
		switch httpErr.Code {
		case fiber.StatusBadRequest:
			kind = ErrInvalidData
		case fiber.StatusUnauthorized:
			kind = ErrUnauthorized
		case fiber.StatusForbidden:
			kind = ErrForbidden
		case fiber.StatusNotFound:
			kind = ErrRecordNotFound
		case fiber.StatusConflict:
			kind = ErrConflict
		case fiber.StatusUnprocessableEntity:
			kind = ErrUnprocessable
		case fiber.StatusTooManyRequests:
			kind = ErrLimitExceeded
		}
	}
	if kind != nil && !errors.Is(err, kind) {
		return errors.Join(kind, err)
	}
	return err
}

func NewAppError(code int, msg string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: msg,
		Err:     err,
	}
}

var (
	// Generic errors
	ErrInternalServer = errors.New("internal server error") // 500
	ErrUnknown        = errors.New("unknown error")         // 500
	ErrTimeout        = errors.New("timeout")               // 504
	ErrUnauthorized   = errors.New("unauthorized")          // 401
	ErrForbidden      = errors.New("forbidden")             // 403
	ErrNotImplemented = errors.New("not implemented")       // 501

	// Database errors]
	ErrRecordNotFound                = errors.New("record not found")                                    // 404
	ErrInvalidTransaction            = errors.New("invalid transaction")                                 // 400
	ErrMissingWhereClause            = errors.New("WHERE conditions required")                           // 400
	ErrUnsupportedRelation           = errors.New("unsupported relations")                               // 400
	ErrPrimaryKeyRequired            = errors.New("primary key required")                                // 400
	ErrModelValueRequired            = errors.New("model value required")                                // 400
	ErrModelAccessibleFieldsRequired = errors.New("model accessible fields required")                    // 400
	ErrSubQueryRequired              = errors.New("sub query required")                                  // 400
	ErrUnsupportData                 = errors.New("unsupported data")                                    // 400
	ErrUnsupportedDriver             = errors.New("unsupported driver")                                  // 400
	ErrRegistered                    = errors.New("registered")                                          // 409
	ErrInvalidField                  = errors.New("invalid field")                                       // 400
	ErrEmptySlice                    = errors.New("empty slice found")                                   // 400
	ErrDryRunModeUnsupported         = errors.New("dry run mode unsupported")                            // 400
	ErrInvalidDB                     = errors.New("invalid db")                                          // 400
	ErrInvalidValue                  = errors.New("invalid value, should be pointer to struct or slice") // 400
	ErrInvalidValueOfLength          = errors.New("invalid association values, length doesn't match")    // 400
	ErrPreloadNotAllowed             = errors.New("preload is not allowed when count is used")           // 400
	ErrDuplicatedKey                 = errors.New("duplicated key not allowed")                          // 409
	ErrForeignKeyViolated            = errors.New("violates foreign key constraint")                     // 409
	ErrCheckConstraintViolated       = errors.New("violates check constraint")                           // 409

	// Validation errors
	ErrInvalidData   = errors.New("invalid data")           // 400
	ErrInvalidID     = errors.New("invalid id")             // 400
	ErrRequiredField = errors.New("required field missing") // 400
	ErrInvalidFormat = errors.New("invalid format")         // 400
	ErrOutOfRange    = errors.New("value out of range")     // 400
	ErrUnprocessable = errors.New("unprocessable entity")   // 422

	// Business logic / domain-specific errors
	ErrAlreadyExists              = errors.New("already exists")                                        // 409
	ErrRegistrationIdentityExists = errors.New("registration email, phone, or username already exists") // 409
	ErrNotAvailable               = errors.New("not available")                                         // 409
	ErrLimitExceeded              = errors.New("limit exceeded")                                        // 429
	ErrOperationDenied            = errors.New("operation denied")                                      // 403
	ErrInvalidOTP                 = errors.New("invalid or expired otp")
	ErrInvalidResetToken          = errors.New("invalid or expired reset token")
	ErrInvalidCredentials         = errors.New("invalid credentials")
	ErrSessionRevoked             = errors.New("session revoked")
	ErrEmailUnavailable           = errors.New("email delivery unavailable")

	// Other errors
	ErrConflict         = errors.New("conflict")            // 409
	ErrDependencyFail   = errors.New("dependency failure")  // 502
	ErrTransactionAbort = errors.New("transaction aborted") // 500
)

// StatusCode maps errors to Fiber HTTP status codes
func StatusCode(err error) int {
	err = Normalize(err)
	var httpErr *fiber.Error
	if errors.As(err, &httpErr) && httpErr.Code >= 400 && httpErr.Code <= 599 {
		return httpErr.Code
	}
	switch {
	// Generic
	case errors.Is(err, ErrInternalServer), errors.Is(err, ErrUnknown), errors.Is(err, ErrTransactionAbort):
		return fiber.StatusInternalServerError
	case errors.Is(err, ErrTimeout):
		return fiber.StatusGatewayTimeout
	case errors.Is(err, ErrEmailUnavailable):
		return fiber.StatusServiceUnavailable
	case errors.Is(err, ErrUnauthorized), errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrInvalidResetToken), errors.Is(err, ErrSessionRevoked):
		return fiber.StatusUnauthorized
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrOperationDenied):
		return fiber.StatusForbidden
	case errors.Is(err, ErrNotImplemented):
		return fiber.StatusNotImplemented

	// Database errors
	case errors.Is(err, ErrRecordNotFound):
		return fiber.StatusNotFound
	case errors.Is(err, ErrDuplicatedKey), errors.Is(err, ErrConflict), errors.Is(err, ErrAlreadyExists),
		errors.Is(err, ErrRegistrationIdentityExists), errors.Is(err, ErrNotAvailable), errors.Is(err, ErrRegistered):
		return fiber.StatusConflict
	case errors.Is(err, ErrDependencyFail):
		return fiber.StatusBadGateway
	case errors.Is(err, ErrInvalidTransaction), errors.Is(err, ErrMissingWhereClause),
		errors.Is(err, ErrUnsupportedRelation), errors.Is(err, ErrPrimaryKeyRequired),
		errors.Is(err, ErrModelValueRequired), errors.Is(err, ErrModelAccessibleFieldsRequired),
		errors.Is(err, ErrSubQueryRequired), errors.Is(err, ErrUnsupportData),
		errors.Is(err, ErrUnsupportedDriver), errors.Is(err, ErrEmptySlice),
		errors.Is(err, ErrDryRunModeUnsupported), errors.Is(err, ErrPreloadNotAllowed), errors.Is(err, ErrInvalidDB),
		errors.Is(err, ErrForeignKeyViolated), errors.Is(err, ErrCheckConstraintViolated):
		return fiber.StatusBadRequest

	// Validation / business logic
	case errors.Is(err, ErrInvalidData), errors.Is(err, ErrInvalidID), errors.Is(err, ErrRequiredField),
		errors.Is(err, ErrInvalidFormat), errors.Is(err, ErrOutOfRange), errors.Is(err, ErrInvalidValue),
		errors.Is(err, ErrInvalidValueOfLength), errors.Is(err, ErrInvalidField):
		return fiber.StatusBadRequest
	case errors.Is(err, ErrUnprocessable):
		return fiber.StatusUnprocessableEntity
	case errors.Is(err, ErrLimitExceeded):
		return fiber.StatusTooManyRequests
	case errors.Is(err, ErrInvalidOTP):
		return fiber.StatusBadRequest

	// Default
	default:
		return fiber.StatusInternalServerError
	}
}

func Code(err error) int {
	err = Normalize(err)
	switch {
	case errors.Is(err, ErrUnauthorized):
		return 1401
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrInvalidResetToken), errors.Is(err, ErrSessionRevoked):
		return 1401
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrOperationDenied):
		return 1403
	case errors.Is(err, ErrRecordNotFound):
		return 1404
	case errors.Is(err, ErrAlreadyExists), errors.Is(err, ErrRegistrationIdentityExists),
		errors.Is(err, ErrConflict), errors.Is(err, ErrDuplicatedKey):
		return 1409
	case errors.Is(err, ErrLimitExceeded):
		return 1429
	case errors.Is(err, ErrInvalidOTP):
		return 1404
	case errors.Is(err, ErrEmailUnavailable):
		return 1503
	case errors.Is(err, ErrInvalidData), errors.Is(err, ErrRequiredField), errors.Is(err, ErrInvalidFormat):
		return 1400
	default:
		return 1000 + StatusCode(err)
	}
}

func Slug(err error) string {
	err = Normalize(err)
	switch {
	case errors.Is(err, ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, ErrInvalidCredentials):
		return "invalid_credentials"
	case errors.Is(err, ErrInvalidResetToken):
		return "invalid_or_expired_reset_token"
	case errors.Is(err, ErrSessionRevoked):
		return "session_revoked"
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrOperationDenied):
		return "forbidden"
	case errors.Is(err, ErrRecordNotFound):
		return "not_found"
	case errors.Is(err, ErrRegistrationIdentityExists):
		return "email_phone_or_username_exists"
	case errors.Is(err, ErrAlreadyExists), errors.Is(err, ErrConflict), errors.Is(err, ErrDuplicatedKey):
		return "conflict"
	case errors.Is(err, ErrLimitExceeded):
		return "rate_limit_exceeded"
	case errors.Is(err, ErrInvalidOTP):
		return "invalid_or_expired_otp"
	case errors.Is(err, ErrEmailUnavailable):
		return "email_delivery_unavailable"
	case errors.Is(err, ErrInvalidData), errors.Is(err, ErrRequiredField), errors.Is(err, ErrInvalidFormat):
		return "invalid_data"
	case errors.Is(err, ErrInvalidID):
		return "invalid_id"
	case errors.Is(err, ErrForeignKeyViolated):
		return "foreign_key_violation"
	case errors.Is(err, ErrCheckConstraintViolated):
		return "check_constraint_violation"
	case errors.Is(err, ErrTimeout):
		return "timeout"
	default:
		switch StatusCode(err) {
		case 400:
			return "invalid_data"
		case 409:
			return "conflict"
		case 422:
			return "unprocessable_entity"
		case 500:
			return "internal_error"
		default:
			return strings.ReplaceAll(strings.ToLower(http.StatusText(StatusCode(err))), " ", "_")
		}
	}
}

func Message(err error) string {
	return MessageForLanguage(err, "en")
}

// MessageForLanguage returns a user-facing error message in Uzbek, Russian, or
// English. It accepts both a plain language code and an Accept-Language value.
func MessageForLanguage(err error, language string) string {
	err = Normalize(err)
	language = localization.ResolveAcceptLanguage(language)

	switch {
	case errors.Is(err, ErrUnauthorized):
		return localized(language, "Hisob ma'lumotlari noto'g'ri", "Неверные учетные данные", "Invalid credentials")
	case errors.Is(err, ErrInvalidCredentials):
		return localized(language, "Login yoki parol noto'g'ri", "Неверные учетные данные", "Invalid credentials")
	case errors.Is(err, ErrInvalidResetToken):
		return localized(language, "Parolni tiklash tokeni noto'g'ri yoki muddati o'tgan", "Недействительный или просроченный токен сброса", "Invalid or expired reset token")
	case errors.Is(err, ErrSessionRevoked):
		return localized(language, "Sessiya bekor qilingan", "Сессия отозвана", "Session revoked")
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrOperationDenied):
		return localized(language, "Ruxsat berilmagan", "Доступ запрещен", "Access denied")
	case errors.Is(err, ErrRecordNotFound):
		return localized(language, "Resurs topilmadi", "Ресурс не найден", "Resource not found")
	case errors.Is(err, ErrRegistrationIdentityExists):
		return localized(language, "Email, telefon raqami yoki foydalanuvchi nomi allaqachon mavjud", "Email, номер телефона или имя пользователя уже существуют", "Email, phone, or username already exists")
	case errors.Is(err, ErrAlreadyExists), errors.Is(err, ErrConflict), errors.Is(err, ErrDuplicatedKey):
		return localized(language, "Ma'lumotlar ziddiyati", "Конфликт данных", "Data conflict")
	case errors.Is(err, ErrLimitExceeded):
		return localized(language, "Juda ko'p so'rov yuborildi", "Слишком много запросов", "Too many requests")
	case errors.Is(err, ErrInvalidOTP):
		return localized(language, "Tasdiqlash kodi noto'g'ri yoki muddati o'tgan", "Неверный или просроченный код", "Invalid or expired verification code")
	case errors.Is(err, ErrEmailUnavailable):
		return localized(language, "Email yuborish vaqtincha ishlamayapti", "Отправка email временно недоступна", "Email delivery is temporarily unavailable")
	case errors.Is(err, ErrInvalidID):
		return localized(language, "ID noto'g'ri", "Некорректный ID", "Invalid ID")
	case errors.Is(err, ErrForeignKeyViolated):
		return localized(language, "Bog'langan ma'lumot mavjud emas yoki ishlatilmoqda", "Связанные данные отсутствуют или используются", "Related data is missing or still in use")
	case errors.Is(err, ErrCheckConstraintViolated):
		return localized(language, "Ma'lumotlar belgilangan shartga mos emas", "Данные не соответствуют ограничению", "Data violates a required constraint")
	case errors.Is(err, ErrTimeout):
		return localized(language, "So'rovni bajarish vaqti tugadi", "Время выполнения запроса истекло", "Request timed out")
	case errors.Is(err, ErrInvalidData), errors.Is(err, ErrRequiredField), errors.Is(err, ErrInvalidFormat):
		return localized(language, "Ma'lumotlar noto'g'ri", "Некорректные данные", "Invalid data")
	default:
		if StatusCode(err) < 500 {
			return localized(language, "So'rovni bajarib bo'lmadi", "Не удалось выполнить запрос", "Request could not be processed")
		}
		return localized(language, "Serverda ichki xatolik yuz berdi", "Внутренняя ошибка сервера", "Internal server error")
	}
}

func localized(language, uz, ru, en string) string {
	switch language {
	case "uz":
		return uz
	case "ru":
		return ru
	default:
		return en
	}
}
