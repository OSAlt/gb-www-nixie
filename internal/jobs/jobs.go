// Package jobs registers the background work that River runs. Workers are
// plain structs so they can be exercised without a database or a running
// client (see the _test files).
package jobs

import (
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/adapters/social"
	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/riverqueue/river"
)

// DefaultQueueMaxWorkers bounds concurrency. The site has few, cheap jobs, so
// a small number is enough and keeps API-quota bursts predictable.
const DefaultQueueMaxWorkers = 4

// Deps is everything the workers need. Fields may be nil: a job whose
// dependency is missing is simply not registered.
type Deps struct {
	Config        *config.Config
	DB            ports.DBService
	UnitOfWork    ports.UnitOfWork
	SocialFetcher *social.Aggregator
}

// NewWorkers registers every available worker.
func NewWorkers(deps Deps) (*river.Workers, error) {
	workers := river.NewWorkers()

	if deps.DB != nil && deps.UnitOfWork != nil && deps.Config != nil && deps.SocialFetcher != nil {
		if err := river.AddWorkerSafely(workers, &SocialMetricsWorker{
			Config:     deps.Config,
			DB:         deps.DB,
			UnitOfWork: deps.UnitOfWork,
			Fetcher:    deps.SocialFetcher,
		}); err != nil {
			return nil, err
		}
	}

	return workers, nil
}

// PeriodicJobs returns the cron-like schedule. It is empty when the worker it
// would enqueue is not registered, so an unconfigured deployment does not
// accumulate jobs of an unknown kind.
func PeriodicJobs(cfg *config.Config, fetcher *social.Aggregator) []*river.PeriodicJob {
	if cfg == nil || fetcher == nil {
		return nil
	}

	interval := time.Duration(cfg.SocialRefreshSeconds) * time.Second

	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(interval),
			func() (river.JobArgs, *river.InsertOpts) {
				return SocialMetricsArgs{}, &river.InsertOpts{
					// The upstream APIs fail transiently; a couple of retries
					// covers that without turning an outage into a quota burn.
					MaxAttempts: 3,
					UniqueOpts:  river.UniqueOpts{ByPeriod: interval},
				}
			},
			// RunOnStart means a deploy or leader change still refreshes, and
			// the worker's freshness guard makes that free when the data is
			// already current.
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	}
}
