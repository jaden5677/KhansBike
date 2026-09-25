package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/khansbikezone/bikezone-api/internal/domain"
)

func TestTranslate(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantKind error
		wantMsg  string
	}{
		{"no_rows", fmt.Errorf("scan: %w", pgx.ErrNoRows), domain.ErrNotFound, "not found"},
		{"known_unique", &pgconn.PgError{Code: "23505", ConstraintName: "products_slug_key"}, domain.ErrConflict,
			"conflict: a product with this slug already exists"},
		{"unknown_unique", &pgconn.PgError{Code: "23505", ConstraintName: "x"}, domain.ErrConflict,
			"conflict: the change conflicts with an existing record"},
		{"still_referenced", &pgconn.PgError{Code: "23503", Message: `update or delete on table "categories" violates foreign key constraint`, TableName: "product_variants"},
			domain.ErrConflict, "conflict: it is still referenced by product variants"},
		{"missing_reference", &pgconn.PgError{Code: "23503", Message: `insert or update on table "products" violates foreign key constraint`},
			domain.ErrValidation, "validation failed: it references a record that does not exist"},
		{"check", &pgconn.PgError{Code: "23514"}, domain.ErrValidation, "validation failed: a value is invalid for its field"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := translate(tc.err)
			if !errors.Is(got, tc.wantKind) {
				t.Errorf("translate() = %v, want kind %v", got, tc.wantKind)
			}
			if got.Error() != tc.wantMsg {
				t.Errorf("message = %q, want %q", got.Error(), tc.wantMsg)
			}
			// The original driver error stays reachable for logs and debugging.
			var pgErr *pgconn.PgError
			if errors.As(tc.err, &pgErr) && !errors.As(got, &pgErr) {
				t.Error("translated error no longer unwraps to the driver error")
			}
		})
	}
	other := errors.New("connection refused")
	if got := translate(other); !errors.Is(got, other) || got.Error() != other.Error() {
		t.Errorf("unrelated error was rewritten: %v", got)
	}
	if translate(nil) != nil {
		t.Error("translate(nil) != nil")
	}
}

func TestAfterCursorMixedDirections(t *testing.T) {
	var b queryBuilder
	keys := []sortKey{{"p.is_featured", true}, {"p.name", false}, {"p.id", false}}
	got := afterCursor(&b, keys, []any{true, "Star Grips", "id-1"})
	want := "((p.is_featured < $1) OR (p.is_featured = $2 AND p.name > $3) OR (p.is_featured = $4 AND p.name = $5 AND p.id > $6))"
	if got != want {
		t.Errorf("afterCursor =\n %s\nwant\n %s", got, want)
	}
	if len(b.args) != 6 || b.args[0] != true || b.args[2] != "Star Grips" {
		t.Errorf("args = %v", b.args)
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`50%_off\`); got != `50\%\_off\\` {
		t.Errorf("escapeLike = %q", got)
	}
}
