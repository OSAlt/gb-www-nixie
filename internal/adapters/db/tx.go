package db

import (
	"context"
	"errors"

	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/jackc/pgx/v5"
)

// errNestedTx is returned when a transaction-scoped adapter is asked to open
// another transaction. Nested transactions are not supported: pass the
// Transaction down instead.
var errNestedTx = errors.New("nested transactions are not supported")

// TxAdapter is an Adapter whose queries run inside an open transaction.
type TxAdapter struct {
	*Adapter
	tx pgx.Tx
}

// PgxTx exposes the underlying transaction for callers that need to put more
// work (such as a job insert) in the same commit.
func (t *TxAdapter) PgxTx() pgx.Tx { return t.tx }

// WithTx runs fn inside a Postgres transaction and commits only when it returns
// nil. Errors and panics roll the transaction back; panics are re-raised.
//
// Transactions are deliberately short-lived: do not perform HTTP calls inside
// fn. Use the default read-committed isolation level unless a flow has a
// read-modify-write invariant that needs pgx.Serializable.
func (a *Adapter) WithTx(ctx context.Context, fn func(ctx context.Context, tx ports.Transaction) error) (err error) {
	if a.pool == nil {
		return errors.New("database adapter has no connection pool")
	}

	tx, err := a.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	// Roll back unless the commit below succeeds. WithoutCancel keeps cleanup
	// working when the request context has already been cancelled.
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()

	txAdapter := &TxAdapter{
		Adapter: &Adapter{queries: New(tx)},
		tx:      tx,
	}
	if err = fn(ctx, txAdapter); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// WithTx on a TxAdapter always fails: a callback already has a transaction and
// nesting is not supported.
func (t *TxAdapter) WithTx(context.Context, func(context.Context, ports.Transaction) error) error {
	return errNestedTx
}

var (
	_ ports.UnitOfWork  = (*Adapter)(nil)
	_ ports.Transaction = (*TxAdapter)(nil)
)
