package group

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

func TestOptionalUUID(t *testing.T) {
	t.Parallel()

	validID := uuid.New()
	tests := []struct {
		name    string
		value   string
		want    uuid.UUID
		wantErr error
	}{
		{name: "empty is optional", value: "", want: uuid.Nil},
		{name: "trims valid uuid", value: "  " + validID.String() + "  ", want: validID},
		{name: "rejects invalid uuid", value: "invalid", wantErr: apperror.ErrInvalidID},
		{name: "rejects nil uuid", value: uuid.Nil.String(), wantErr: apperror.ErrInvalidID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := optionalUUID(tt.value)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("optionalUUID(%q) error = %v, want %v", tt.value, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("optionalUUID(%q) = %s, want %s", tt.value, got, tt.want)
			}
		})
	}
}
