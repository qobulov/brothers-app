package group

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestResourceResponsesOmitAuditTimestamps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
	}{
		{
			name: "location",
			value: Location{
				ID:        uuid.New(),
				GroupID:   uuid.New(),
				Name:      "Tashkent",
				CreatedBy: uuid.New(),
			},
		},
		{
			name:  "customer",
			value: Customer{ID: uuid.New(), Phone: "+998901234567"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("marshal response: %v", err)
			}
			var response map[string]any
			if err := json.Unmarshal(encoded, &response); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			for _, field := range []string{"created_at", "updated_at", "deleted_at"} {
				if _, exists := response[field]; exists {
					t.Fatalf("response contains forbidden field %q: %s", field, encoded)
				}
			}
		})
	}
}
