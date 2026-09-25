//go:build integration

package jobs_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/khansbikezone/bikezone-api/internal/jobs"
	"github.com/khansbikezone/bikezone-api/internal/service"
	"github.com/khansbikezone/bikezone-api/internal/store"
	"github.com/khansbikezone/bikezone-api/internal/testutil"
)

type harness struct {
	t      *testing.T
	ctx    context.Context
	store  *store.Store
	runner *jobs.Runner
	db     *pgx.Conn // direct access, to inspect and fast-forward the queue
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	a := testutil.App(t, nil)
	db, err := pgx.Connect(ctx, a.Config.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(ctx) })
	return &harness{t: t, ctx: ctx, store: a.Store, runner: jobs.NewRunner(a.Store, slog.New(slog.NewTextHandler(io.Discard, nil)), 1), db: db}
}

func (h *harness) enqueue(kind string) uuid.UUID {
	h.t.Helper()
	var id uuid.UUID
	if err := h.db.QueryRow(h.ctx, `INSERT INTO jobs (kind, max_attempts) VALUES ($1, 3) RETURNING id`, kind).Scan(&id); err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *harness) state(id uuid.UUID) (state string, attempts int) {
	h.t.Helper()
	if err := h.db.QueryRow(h.ctx, `SELECT state, attempts FROM jobs WHERE id = $1`, id).Scan(&state, &attempts); err != nil {
		h.t.Fatal(err)
	}
	return state, attempts
}

// next processes one job, first making every queued job due now (retries
// are otherwise scheduled seconds into the future).
func (h *harness) next() bool {
	h.t.Helper()
	if _, err := h.db.Exec(h.ctx, `UPDATE jobs SET run_after = now() WHERE state = 'queued'`); err != nil {
		h.t.Fatal(err)
	}
	ran, err := h.runner.ProcessNext(h.ctx, "test")
	if err != nil {
		h.t.Fatal(err)
	}
	return ran
}

func TestRetriesThenSucceeds(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	calls := 0
	h.runner.Handle("flaky", func(context.Context, []byte) error {
		calls++
		if calls < 3 {
			return errors.New("temporary failure")
		}
		return nil
	})
	id := h.enqueue("flaky")
	for h.next() {
	}
	if st, n := h.state(id); st != "done" || n != 3 || calls != 3 {
		t.Errorf("state %s after %d attempts (%d calls), want done after 3", st, n, calls)
	}
}

func TestPermanentPanickingAndUnknownJobs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.runner.Handle("bad-payload", func(context.Context, []byte) error { return jobs.Permanent(errors.New("malformed")) })
	h.runner.Handle("always-fails", func(context.Context, []byte) error { return errors.New("down") })
	h.runner.Handle("panics", func(context.Context, []byte) error { panic("bug") })

	permanent, exhausted, panicked, unknown := h.enqueue("bad-payload"), h.enqueue("always-fails"), h.enqueue("panics"), h.enqueue("no-such-kind")
	for h.next() {
	}
	for id, want := range map[uuid.UUID]string{permanent: "failed", exhausted: "dead", panicked: "dead", unknown: "failed"} {
		if st, n := h.state(id); st != want {
			t.Errorf("job %s: state %s after %d attempts, want %s", id, st, n, want)
		}
	}
	if _, n := h.state(permanent); n != 1 {
		t.Errorf("permanent failure was retried (%d attempts)", n)
	}
}

func TestReaperRequeuesThenDeadLettersStuckJobs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	id := h.enqueue("stuck")
	// A worker claimed it and died 15 minutes ago.
	if _, err := h.db.Exec(h.ctx, `UPDATE jobs SET state = 'running', attempts = 1, locked_at = now() - interval '15 minutes' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	reap := func() {
		if _, err := h.store.ReapStuckJobs(h.ctx); err != nil { // what the maintenance loop runs each minute
			t.Fatal(err)
		}
	}
	reap()
	if st, _ := h.state(id); st != "queued" {
		t.Fatalf("stuck job with attempts left: state %s, want queued", st)
	}
	if _, err := h.db.Exec(h.ctx, `UPDATE jobs SET state = 'running', attempts = max_attempts, locked_at = now() - interval '15 minutes' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	reap()
	if st, _ := h.state(id); st != "dead" {
		t.Errorf("stuck job out of attempts: state %s, want dead", st)
	}
}

func TestReindexJobsCoalesce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := testutil.App(t, nil)
	db, err := pgx.Connect(ctx, a.Config.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close(ctx) }()
	for i := 0; i < 5; i++ { // a burst of edits, each enqueueing a reindex
		if _, err := db.Exec(ctx, `INSERT INTO jobs (kind) VALUES ($1)`, service.JobReindexSearch); err != nil {
			t.Fatal(err)
		}
	}
	testutil.DrainJobs(t, a)
	var done, left int
	if err := db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE state = 'done'), count(*) FILTER (WHERE state = 'queued') FROM jobs`).Scan(&done, &left); err != nil {
		t.Fatal(err)
	}
	if done != 1 || left != 0 {
		t.Errorf("reindex ran %d times with %d left queued, want once with none left", done, left)
	}
}
