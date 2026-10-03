// Package paging holds the limit and offset rules shared by every list endpoint.
package paging

import "github.com/qobulov/brothers-app/pkg/apperror"

const (
	DefaultLimit = 20
	MaxLimit     = 100
	MaxOffset    = 10000
)

// Page with Limit 0 uses the default page size.
type Page struct {
	Limit  int
	Offset int
}

// Valid applies the default limit and rejects a page outside the allowed range.
func (p Page) Valid() (Page, error) {
	if p.Limit == 0 {
		p.Limit = DefaultLimit
	}
	if p.Limit < 1 || p.Limit > MaxLimit || p.Offset < 0 || p.Offset > MaxOffset {
		return Page{}, apperror.PageOutOfRange(MaxLimit, MaxOffset)
	}
	return p, nil
}
