// Package jobs holds River jobs that belong to no feature module.
package jobs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// EventPartitionsArgs creates the events table's monthly partitions ahead.
type EventPartitionsArgs struct{}

// Kind implements river.JobArgs.
func (EventPartitionsArgs) Kind() string { return "event_partitions" }

// EventPartitionsWorker runs ensure_event_partitions(2).
type EventPartitionsWorker struct {
	river.WorkerDefaults[EventPartitionsArgs]
	DB *pgxpool.Pool
}

// Work implements river.Worker.
func (w *EventPartitionsWorker) Work(ctx context.Context, _ *river.Job[EventPartitionsArgs]) error {
	if _, err := w.DB.Exec(ctx, `SELECT ensure_event_partitions(2)`); err != nil {
		return fmt.Errorf("ensure_event_partitions: %w", err)
	}
	return nil
}
