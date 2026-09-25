// Package store is the persistence layer: the pgx connection pool, the sqlc
// generated queries (internal/store/gen), the hand-written dynamic catalogue
// queries, and conversions between database rows and domain types.
//
// Every error that leaves a query has already been translated into the domain
// vocabulary (domain.ErrNotFound, ErrConflict, ErrValidation) by a thin adapter
// wrapped around the connection, so services never inspect pgx or pgconn
// errors and the HTTP layer maps errors to status codes in exactly one place.
package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// Queries is the full query surface bound to one connection or transaction:
// every generated sqlc query plus the hand-written ones defined in this
// package. Services receive a *Queries and need not care which it is bound to.
type Queries struct {
	*gen.Queries
	db gen.DBTX
}

func newQueries(db gen.DBTX) *Queries {
	t := translatingDB{inner: db}
	return &Queries{Queries: gen.New(t), db: t}
}

// Store owns the pool. Its embedded Queries run on the pool (each statement
// in its own implicit transaction); InTx runs a function in one transaction.
type Store struct {
	*Queries
	pool *pgxpool.Pool
}

// New wraps an open pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{Queries: newQueries(pool), pool: pool}
}

// Ping checks the database is reachable; it backs the readiness probe.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// InTx runs fn inside a single transaction, committing if fn returns nil and
// rolling back otherwise. Keep fn free of slow external I/O (email, blob
// uploads): a transaction holds row locks until it ends.
func (s *Store) InTx(ctx context.Context, fn func(q *Queries) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(newQueries(tx))
	})
}

// translatingDB implements gen.DBTX by delegating to a pool or transaction
// and translating errors on the way out.
type translatingDB struct{ inner gen.DBTX }

func (d translatingDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tag, err := d.inner.Exec(ctx, sql, args...)
	return tag, translate(err)
}

func (d translatingDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	rows, err := d.inner.Query(ctx, sql, args...)
	if err != nil {
		return nil, translate(err)
	}
	return translatingRows{rows}, nil
}

func (d translatingDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return translatingRow{d.inner.QueryRow(ctx, sql, args...)}
}

func (d translatingDB) CopyFrom(ctx context.Context, table pgx.Identifier, cols []string, src pgx.CopyFromSource) (int64, error) {
	n, err := d.inner.CopyFrom(ctx, table, cols, src)
	return n, translate(err)
}

// translatingRows covers errors that only surface while iterating (a query
// error arrives with the first row, not from Query itself).
type translatingRows struct{ pgx.Rows }

func (r translatingRows) Err() error             { return translate(r.Rows.Err()) }
func (r translatingRows) Scan(dest ...any) error { return translate(r.Rows.Scan(dest...)) }

type translatingRow struct{ pgx.Row }

func (r translatingRow) Scan(dest ...any) error { return translate(r.Row.Scan(dest...)) }

// dbError is a translated driver error. Its message is safe to show a client;
// it matches its domain sentinel under errors.Is and still unwraps to the
// original driver error so logs and debugging keep the full detail.
type dbError struct {
	kind  error
	msg   string
	cause error
}

func (e *dbError) Error() string {
	if e.msg == "" {
		return e.kind.Error()
	}
	return e.kind.Error() + ": " + e.msg
}

func (e *dbError) Unwrap() []error { return []error{e.kind, e.cause} }

// Postgres error codes this layer understands.
const (
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
	codeCheckViolation      = "23514"
	codeNotNullViolation    = "23502"
	codeInvalidText         = "22P02"
	codeStringTooLong       = "22001"
	codeNumericOutOfRange   = "22003"
)

// constraintMessages turns the unique constraints a client can realistically
// hit into readable explanations. Unlisted constraints get a generic message.
var constraintMessages = map[string]string{
	"products_slug_key":                         "a product with this slug already exists",
	"categories_slug_key":                       "a category with this slug already exists",
	"brands_slug_key":                           "a brand with this slug already exists",
	"attributes_key_key":                        "an attribute with this key already exists",
	"attribute_options_attribute_id_value_key":  "this attribute already has an option with that value",
	"product_variants_product_id_sku_key":       "SKUs must be unique within a product",
	"ux_variant_default":                        "a product can have only one default variant",
	"ux_product_media":                          "this image is already attached to the product",
	"users_email_key":                           "a user with this email already exists",
	"prices_variant_id_tier_effective_from_key": "a price for this tier and date already exists",
}

func translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// Services prefix what was being looked up: `product "x": not found`.
		return &dbError{kind: domain.ErrNotFound, cause: err}
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case codeUniqueViolation:
		msg, ok := constraintMessages[pgErr.ConstraintName]
		if !ok {
			msg = "the change conflicts with an existing record"
		}
		return &dbError{kind: domain.ErrConflict, msg: msg, cause: err}
	case codeForeignKeyViolation:
		// Deleting (or re-keying) a row that others still reference is a
		// conflict with current state; inserting a reference to a row that
		// does not exist is invalid input.
		if strings.HasPrefix(pgErr.Message, "update or delete") {
			referrer := strings.ReplaceAll(pgErr.TableName, "_", " ")
			return &dbError{kind: domain.ErrConflict, msg: "it is still referenced by " + referrer, cause: err}
		}
		return &dbError{kind: domain.ErrValidation, msg: "it references a record that does not exist", cause: err}
	case codeCheckViolation, codeNotNullViolation, codeInvalidText, codeStringTooLong, codeNumericOutOfRange:
		return &dbError{kind: domain.ErrValidation, msg: "a value is invalid for its field", cause: err}
	default:
		return err
	}
}
