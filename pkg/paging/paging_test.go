package paging

import (
	"errors"
	"testing"

	"github.com/qobulov/brothers-app/pkg/apperror"
)

func TestValid(t *testing.T) {
	if got, err := (Page{}).Valid(); err != nil || got != (Page{Limit: DefaultLimit}) {
		t.Fatalf("empty page = %+v, %v; want the default limit", got, err)
	}
	if got, err := (Page{Limit: MaxLimit, Offset: MaxOffset}).Valid(); err != nil || got.Limit != MaxLimit || got.Offset != MaxOffset {
		t.Fatalf("largest page = %+v, %v; want it accepted unchanged", got, err)
	}
	for _, page := range []Page{{Limit: -1}, {Limit: MaxLimit + 1}, {Offset: -1}, {Offset: MaxOffset + 1}} {
		if _, err := page.Valid(); !errors.Is(err, apperror.ErrInvalidData) {
			t.Fatalf("page %+v: err = %v, want invalid data", page, err)
		}
	}
}
