package jobs

import (
	"context"

	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

var _ ports.JobQueue = (*Queue)(nil)

// Queue enqueues River jobs. InsertTx is the transactional variant: the job
// commits (or rolls back) with the caller's business writes.
type Queue struct {
	client *river.Client[pgx.Tx]
}

func NewQueue(client *river.Client[pgx.Tx]) *Queue {
	return &Queue{client: client}
}

func (q *Queue) Insert(ctx context.Context, args ports.JobArgs) error {
	_, err := q.client.Insert(ctx, args, nil)
	return err
}

func (q *Queue) InsertTx(ctx context.Context, tx pgx.Tx, args ports.JobArgs) error {
	_, err := q.client.InsertTx(ctx, tx, args, nil)
	return err
}
