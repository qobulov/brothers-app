package database

import (
	"context"
	"strings"
	"testing"
)

func TestEveryTableUsesUUIDAndAuditColumns(t *testing.T) {
	pool, cleanup := SetupTestDB(t)
	defer cleanup()

	rows, err := pool.Query(context.Background(), `
		SELECT tables.table_name,
		       COALESCE(id.data_type, ''), COALESCE(id.column_default, ''),
		       COALESCE(created.data_type, ''), COALESCE(created.is_nullable, ''), COALESCE(created.column_default, ''),
		       COALESCE(updated.data_type, ''), COALESCE(updated.is_nullable, ''), COALESCE(updated.column_default, ''),
		       COALESCE(deleted.data_type, ''), COALESCE(deleted.is_nullable, '')
		FROM information_schema.tables tables
		LEFT JOIN information_schema.columns id
		  ON id.table_schema = tables.table_schema AND id.table_name = tables.table_name AND id.column_name = 'id'
		LEFT JOIN information_schema.columns created
		  ON created.table_schema = tables.table_schema AND created.table_name = tables.table_name AND created.column_name = 'created_at'
		LEFT JOIN information_schema.columns updated
		  ON updated.table_schema = tables.table_schema AND updated.table_name = tables.table_name AND updated.column_name = 'updated_at'
		LEFT JOIN information_schema.columns deleted
		  ON deleted.table_schema = tables.table_schema AND deleted.table_name = tables.table_name AND deleted.column_name = 'deleted_at'
		WHERE tables.table_schema = current_schema() AND tables.table_type = 'BASE TABLE'
		ORDER BY tables.table_name
	`)
	if err != nil {
		t.Fatalf("query schema metadata: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
		var table, idType, idDefault string
		var createdType, createdNullable, createdDefault string
		var updatedType, updatedNullable, updatedDefault string
		var deletedType, deletedNullable string
		if err := rows.Scan(
			&table, &idType, &idDefault,
			&createdType, &createdNullable, &createdDefault,
			&updatedType, &updatedNullable, &updatedDefault,
			&deletedType, &deletedNullable,
		); err != nil {
			t.Fatalf("scan schema metadata: %v", err)
		}

		if idType != "uuid" || !strings.Contains(idDefault, "gen_random_uuid()") {
			t.Errorf("%s.id: type/default = %q/%q, want uuid/gen_random_uuid()", table, idType, idDefault)
		}
		if createdType != "timestamp with time zone" || createdNullable != "NO" || createdDefault == "" {
			t.Errorf("%s.created_at: type/nullable/default = %q/%q/%q", table, createdType, createdNullable, createdDefault)
		}
		if updatedType != "timestamp with time zone" || updatedNullable != "NO" || updatedDefault == "" {
			t.Errorf("%s.updated_at: type/nullable/default = %q/%q/%q", table, updatedType, updatedNullable, updatedDefault)
		}
		if deletedType != "timestamp with time zone" || deletedNullable != "YES" {
			t.Errorf("%s.deleted_at: type/nullable = %q/%q", table, deletedType, deletedNullable)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema metadata: %v", err)
	}
	if count == 0 {
		t.Fatal("no application tables found")
	}
}
