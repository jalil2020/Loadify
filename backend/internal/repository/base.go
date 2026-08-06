// Package repository implements the domain persistence ports with GORM.
// Nothing above this package imports gorm.
package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
)

// txContextKey carries an open transaction through the context so a usecase can
// call several repositories inside one atomic unit without passing *gorm.DB around.
type txContextKey struct{}

// Base gives every repository its connection resolution and error translation.
type Base struct{ db *gorm.DB }

// NewBase builds the shared repository base.
func NewBase(db *gorm.DB) Base { return Base{db: db} }

// DB returns the transaction bound to ctx, or the pooled connection.
func (b Base) DB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(txContextKey{}).(*gorm.DB); ok {
		return tx.WithContext(ctx)
	}
	return b.db.WithContext(ctx)
}

// TxManager implements domain.TxManager.
type TxManager struct{ db *gorm.DB }

// NewTxManager builds the transaction manager.
func NewTxManager(db *gorm.DB) domain.TxManager { return &TxManager{db: db} }

// WithinTransaction runs fn inside a transaction, committing on success and
// rolling back on error or panic. Nested calls join the outer transaction
// instead of opening a second one.
func (t *TxManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if _, ok := ctx.Value(txContextKey{}).(*gorm.DB); ok {
		return fn(ctx)
	}
	return t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txContextKey{}, tx))
	})
}

// translate converts driver and ORM errors into apperror values. Handlers never
// see a raw SQL error, and clients never see a table name.
func translate(err error, resource string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperror.ErrNotFound.WithMessage(resource + " not found").Wrap(err)
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "SQLSTATE 23505"): // unique_violation
		return apperror.ErrConflict.
			WithMessage(resource + " already exists").Wrap(err)
	case strings.Contains(msg, "SQLSTATE 23503"): // foreign_key_violation
		return apperror.ErrBadRequest.
			WithMessage("Referenced " + resource + " does not exist").Wrap(err)
	case strings.Contains(msg, "SQLSTATE 23514"): // check_violation
		return apperror.ErrValidation.
			WithMessage("The submitted " + resource + " data breaks a constraint").Wrap(err)
	case strings.Contains(msg, "SQLSTATE 40001"), // serialization_failure
		strings.Contains(msg, "SQLSTATE 40P01"): // deadlock_detected
		return apperror.ErrConflict.
			WithMessage("The request conflicted with another operation, retry").Wrap(err)
	case strings.Contains(msg, "SQLSTATE 57014"): // query_canceled
		return apperror.ErrUnavailable.WithMessage("The request took too long").Wrap(err)
	default:
		return apperror.ErrInternal.Wrap(fmt.Errorf("%s: %w", resource, err))
	}
}

// isUniqueViolation reports whether the error is a Postgres unique-constraint
// breach. Callers use it to turn a race into a precise domain error rather than
// the generic "already exists" from translate.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}

// applyOrder appends a validated ORDER BY clause.
func applyOrder(q *gorm.DB, order string) *gorm.DB {
	if order == "" {
		return q
	}
	return q.Order(order)
}

// applyPage appends LIMIT/OFFSET.
func applyPage(q *gorm.DB, offset, limit int) *gorm.DB {
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	return q
}
