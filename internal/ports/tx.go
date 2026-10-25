package ports

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Transaction is a DBService bound to an open Postgres transaction. Every
// write made through it commits or rolls back together.
type Transaction interface {
	DBService

	// PgxTx exposes the underlying transaction so background work can be
	// enqueued in the same commit (for example River's InsertTx). Callers must
	// not commit or roll it back themselves.
	PgxTx() pgx.Tx
}

// UnitOfWork runs fn inside a database transaction. It commits when fn
// returns nil and rolls back when fn returns an error or panics.
type UnitOfWork interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, tx Transaction) error) error
}
