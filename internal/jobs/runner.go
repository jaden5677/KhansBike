// Package jobs runs background work from the durable Postgres job queue.
//
// Jobs are rows in the jobs table, enqueued in the same transaction as the
// change that needs them (see store.Queries.Enqueue) or by database triggers.
// Workers claim them with FOR UPDATE SKIP LOCKED, so any number of workers, in
// the API process or the standalone worker binary, share the queue safely.
// A failed job is retried with exponential backoff until it is dead-lettered;
// a job whose worker crashed is reclaimed by the maintenance loop.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/store"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// Handler performs one job. Returning an error schedules a retry, unless the
// error is wrapped with Permanent. Delivery is at-least-once (a worker can
// crash after finishing but before recording it), so handlers must be
// idempotent.
type Handler func(ctx context.Context, payload []byte) error

type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent marks an error as not worth retrying (a malformed payload, an
// undecodable image); the job is failed immediately instead.
func Permanent(err error) error { return permanentError{err} }

// Tunables. The job timeout must stay below the 10-minute lease after which
// ReapStuckJobs assumes a worker died.
const (
	pollInterval      = time.Second
	jobTimeout        = 5 * time.Minute
	shutdownGrace     = 20 * time.Second
	reapInterval      = time.Minute
	retentionInterval = time.Hour
)

// Runner claims and executes jobs.
type Runner struct {
	store       *store.Store
	log         *slog.Logger
	concurrency int
	handlers    map[string]Handler
	name        string
}

// NewRunner creates a runner with the given number of concurrent workers.
func NewRunner(st *store.Store, log *slog.Logger, concurrency int) *Runner {
	host, _ := os.Hostname()
	return &Runner{
		store:       st,
		log:         log,
		concurrency: max(1, concurrency),
		handlers:    map[string]Handler{},
		name:        fmt.Sprintf("%s/%d", host, os.Getpid()),
	}
}

// Handle registers the handler for a job kind. Register everything before Run.
func (r *Runner) Handle(kind string, h Handler) { r.handlers[kind] = h }

// Run processes jobs until ctx is cancelled. In-flight jobs then get a grace
// period to finish; any still running after it are cancelled, and the queue
// retries them later. Run returns once every worker has stopped.
func (r *Runner) Run(ctx context.Context) {
	// Jobs run on a context detached from ctx, so a shutdown signal stops new
	// claims immediately without killing the work already under way.
	jobCtx, cancelJobs := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelJobs()

	var wg sync.WaitGroup
	for i := 0; i < r.concurrency; i++ {
		wg.Add(1)
		go func(worker string) {
			defer wg.Done()
			r.work(ctx, jobCtx, worker)
		}(fmt.Sprintf("%s#%d", r.name, i))
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.maintain(ctx)
	}()
	r.log.Info("job runner started", "workers", r.concurrency)

	<-ctx.Done()
	stopped := make(chan struct{})
	go func() {
		wg.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(shutdownGrace):
		r.log.Warn("jobs still running after grace period; cancelling them for retry")
		cancelJobs()
		<-stopped
	}
	r.log.Info("job runner stopped")
}

// work is one worker's loop: claim, run, repeat; sleep only when idle.
func (r *Runner) work(ctx, jobCtx context.Context, worker string) {
	for ctx.Err() == nil {
		ran, err := r.ProcessNext(jobCtx, worker)
		if err != nil {
			r.log.Error("job queue error", "worker", worker, "error", err)
		}
		if ran && err == nil {
			continue // there may be more work queued
		}
		select {
		case <-ctx.Done():
		case <-time.After(pollInterval):
		}
	}
}

// ProcessNext claims one runnable job and runs it to completion, reporting
// whether a job was found. Errors are queue failures; a job's own failure is
// recorded on the job (retry or fail) and is not returned.
func (r *Runner) ProcessNext(ctx context.Context, worker string) (bool, error) {
	job, err := r.store.ClaimJob(ctx, worker)
	if errors.Is(err, domain.ErrNotFound) { // the queue is empty
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim job: %w", err)
	}
	log := r.log.With("job_id", job.ID, "kind", job.Kind, "attempt", job.Attempts)

	started := time.Now()
	runErr := r.run(ctx, job)
	switch {
	case runErr == nil:
		log.Info("job done", "duration", time.Since(started))
		return true, r.store.CompleteJob(ctx, job.ID)
	case errors.As(runErr, new(permanentError)):
		log.Error("job failed permanently", "error", runErr)
		return true, r.store.FailJob(ctx, gen.FailJobParams{ID: job.ID, LastError: runErr.Error()})
	default:
		log.Warn("job failed; will retry unless out of attempts", "error", runErr, "max_attempts", job.MaxAttempts)
		return true, r.store.RetryJob(ctx, gen.RetryJobParams{ID: job.ID, LastError: runErr.Error()})
	}
}

// run executes a job's handler with a timeout, converting a panic into an
// error so one bad job cannot take down the process.
func (r *Runner) run(ctx context.Context, job gen.Job) (err error) {
	h, ok := r.handlers[job.Kind]
	if !ok {
		return Permanent(fmt.Errorf("no handler registered for job kind %q", job.Kind))
	}
	ctx, cancel := context.WithTimeout(ctx, jobTimeout)
	defer cancel()
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("job panicked: %v\n%s", p, debug.Stack())
		}
	}()
	return h(ctx, job.Payload)
}

// maintain runs the periodic housekeeping: reclaiming jobs from crashed
// workers every minute, and hourly retention sweeps. Both also run once at
// startup so a restart cleans up promptly.
func (r *Runner) maintain(ctx context.Context) {
	reap := time.NewTicker(reapInterval)
	defer reap.Stop()
	retention := time.NewTicker(retentionInterval)
	defer retention.Stop()
	r.reap(ctx)
	r.sweep(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-reap.C:
			r.reap(ctx)
		case <-retention.C:
			r.sweep(ctx)
		}
	}
}

func (r *Runner) reap(ctx context.Context) {
	if n, err := r.store.ReapStuckJobs(ctx); err != nil {
		r.log.Error("reap stuck jobs", "error", err)
	} else if n > 0 {
		r.log.Warn("reclaimed jobs from a stalled worker", "count", n)
	}
}

// sweep deletes rows that have outlived their purpose.
func (r *Runner) sweep(ctx context.Context) {
	tasks := []struct {
		name string
		fn   func(context.Context) (int64, error)
	}{
		{"expired sessions", r.store.DeleteExpiredSessions},
		{"stale pairing codes", r.store.DeleteStalePairingCodes},
		{"finished jobs", r.store.DeleteFinishedJobs},
	}
	for _, t := range tasks {
		n, err := t.fn(ctx)
		if err != nil {
			r.log.Error("retention sweep failed", "task", t.name, "error", err)
			continue
		}
		if n > 0 {
			r.log.Info("retention sweep", "task", t.name, "rows", n)
		}
	}
}
