// Package request reads the parts of an HTTP request that every handler needs.
package request

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/paging"
)

// UserID returns the authenticated user set by the session middleware.
func UserID(c *fiber.Ctx) (uuid.UUID, error) {
	id, ok := c.Locals("auth_user_id").(uuid.UUID)
	if !ok || id == uuid.Nil {
		return uuid.Nil, apperror.ErrUnauthorized
	}
	return id, nil
}

// PathUUID reads a UUID path parameter such as :groupID.
func PathUUID(c *fiber.Ctx, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, apperror.ErrInvalidID
	}
	return id, nil
}

// Page reads limit and offset. A missing value is 0, so Page.Valid applies the
// default; the range is checked there.
func Page(c *fiber.Ctx) (paging.Page, error) {
	limit, err := optionalInt(c, "limit")
	if err != nil {
		return paging.Page{}, err
	}
	offset, err := optionalInt(c, "offset")
	if err != nil {
		return paging.Page{}, err
	}
	return paging.Page{Limit: limit, Offset: offset}, nil
}

func optionalInt(c *fiber.Ctx, name string) (int, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, apperror.NotAnInteger(name)
	}
	return value, nil
}
