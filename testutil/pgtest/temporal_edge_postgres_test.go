//go:build integration

package pgtest_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/models"
	"github.com/mrz1836/go-foundation/testutil/pgtest"
)

// lifecycleWait bounds every wait on another transaction in these tests.
const lifecycleWait = 10 * time.Second

// lifecycleEdgeID is the typed ID of lifecycleEdge.
type lifecycleEdgeID string

// lifecycleOwnerID is the typed ID of the party a lifecycleEdge belongs to.
type lifecycleOwnerID string

// lifecycleEdge is an owner-scoped edge model used to exercise the lifecycle
// helpers against PostgreSQL's uuid columns, row locks, and isolation.
type lifecycleEdge struct {
	models.TemporalEdge[lifecycleEdgeID]

	OwnerID lifecycleOwnerID `gorm:"type:uuid;not null;index"`
	Label   string
}

// newLifecycleEdgeDB returns a private schema holding lifecycle_edges, with a
// partial unique index that allows one active edge per owner.
func newLifecycleEdgeDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := pgtest.NewPostgresIsolatedDB(t)
	require.NoError(t, db.AutoMigrate(&lifecycleEdge{}))
	require.NoError(t, db.Exec(
		"CREATE UNIQUE INDEX lifecycle_edges_one_active ON lifecycle_edges (owner_id) WHERE "+models.ActiveEdgePredicate,
	).Error)

	return db
}

// newLifecycleEdge returns an unsaved active edge for owner.
func newLifecycleEdge(owner lifecycleOwnerID, label string, confidence float64) *lifecycleEdge {
	return &lifecycleEdge{
		TemporalEdge: models.TemporalEdge[lifecycleEdgeID]{
			ValidFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Source:     "county-registry",
			Confidence: &confidence,
		},
		OwnerID: owner,
		Label:   label,
	}
}

// storedLifecycleEdge reads one stored edge.
func storedLifecycleEdge(t *testing.T, db *gorm.DB, id lifecycleEdgeID) lifecycleEdge {
	t.Helper()

	var edge lifecycleEdge
	require.NoError(t, db.Take(&edge, "id = ?", string(id)).Error)

	return edge
}

// backendPID returns the PostgreSQL backend of the transaction ctx carries.
func backendPID(ctx context.Context) (int64, error) {
	var pid int64
	err := models.DBFrom(ctx, nil).Raw("SELECT pg_backend_pid()").Scan(&pid).Error

	return pid, err
}

// waitUntilBlocked waits until backend pid waits on a lock. The count is
// scoped to pid because every test in this binary shares one server, and
// another test's lock must not satisfy it.
func waitUntilBlocked(t *testing.T, db *gorm.DB, pid int64) {
	t.Helper()

	require.Eventually(t, func() bool {
		var waiting int64
		if err := db.Raw("SELECT count(*) FROM pg_locks WHERE NOT granted AND pid = ?", pid).Scan(&waiting).Error; err != nil {
			return false
		}

		return waiting >= 1
	}, lifecycleWait, 20*time.Millisecond, "backend %d never waited on a lock", pid)
}

// receive returns the next value from ch, failing the test when none arrives
// in time or when done reports that the sending transaction ended first.
func receive[V any](t *testing.T, ch <-chan V, done <-chan error, what string) V {
	t.Helper()

	select {
	case v := <-ch:
		return v
	case err := <-done:
		require.FailNowf(t, "transaction ended early", "%s: %v", what, err)
	case <-time.After(lifecycleWait):
		require.FailNow(t, "timed out waiting", what)
	}

	var zero V

	return zero
}

func TestTemporalEdgeLifecycle_PostgresRoundTrip(t *testing.T) {
	db := newLifecycleEdgeDB(t)
	transactor := models.NewTransactor(db)
	now := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	ctx := models.WithClock(t.Context(), models.NewFixedClock(now))

	ownerA := lifecycleOwnerID(models.NewID())
	ownerB := lifecycleOwnerID(models.NewID())
	scopeA := models.EdgeWhere("owner_id = ?", ownerA)
	scopeB := models.EdgeWhere("owner_id = ?", ownerB)

	original := newLifecycleEdge(ownerA, "original", 0.9)
	require.NoError(t, db.Create(original).Error)
	other := newLifecycleEdge(ownerB, "other", 0.8)
	require.NoError(t, db.Create(other).Error)

	// Correct A's edge.
	corrected := newLifecycleEdge(ownerA, "corrected", 0.95)
	require.NoError(t, transactor.WithinTx(ctx, func(ctx context.Context) error {
		return models.SupersedeEdge(ctx, original.ID, corrected, scopeA)
	}))

	// A scope that names another owner matches nothing.
	err := transactor.WithinTx(ctx, func(ctx context.Context) error {
		_, endErr := models.EndEdge[lifecycleEdge](ctx, corrected.ID, scopeB)

		return endErr
	})
	require.ErrorIs(t, err, models.ErrNotFound)

	// End A's corrected edge, and take down B's.
	var closed *lifecycleEdge

	require.NoError(t, transactor.WithinTx(ctx, func(ctx context.Context) error {
		var endErr error
		closed, endErr = models.EndEdge[lifecycleEdge](ctx, corrected.ID, scopeA)

		return endErr
	}))
	require.NoError(t, transactor.WithinTx(ctx, func(ctx context.Context) error {
		return models.SuppressEdge[lifecycleEdge](ctx, other.ID, scopeB)
	}))

	var count int64
	require.NoError(t, db.Model(&lifecycleEdge{}).Count(&count).Error)
	assert.Equal(t, int64(4), count, "original, corrected, its closed copy, and other")

	storedOriginal := storedLifecycleEdge(t, db, original.ID)
	require.NotNil(t, storedOriginal.SupersededByID)
	assert.Equal(t, corrected.ID, *storedOriginal.SupersededByID)
	assert.Nil(t, storedOriginal.ValidTo)

	storedCorrected := storedLifecycleEdge(t, db, corrected.ID)
	require.NotNil(t, storedCorrected.SupersededByID)
	assert.Equal(t, closed.ID, *storedCorrected.SupersededByID)
	assert.Nil(t, storedCorrected.ValidTo, "ending an edge never writes its valid_to")

	storedClosed := storedLifecycleEdge(t, db, closed.ID)
	require.NotNil(t, storedClosed.ValidTo)
	assert.True(t, now.Equal(*storedClosed.ValidTo))
	assert.True(t, now.Equal(storedClosed.RecordedAt))
	assert.True(t, storedCorrected.ValidFrom.Equal(storedClosed.ValidFrom))
	assert.Equal(t, ownerA, storedClosed.OwnerID)
	assert.Equal(t, "corrected", storedClosed.Label)
	require.NotNil(t, storedClosed.Confidence)
	assert.InDelta(t, 0.95, *storedClosed.Confidence, 1e-9)
	assert.Nil(t, storedClosed.SupersededByID)

	storedOther := storedLifecycleEdge(t, db, other.ID)
	require.NotNil(t, storedOther.SuppressedAt)
	assert.True(t, now.Equal(*storedOther.SuppressedAt))
	assert.Nil(t, storedOther.SupersededByID)

	var active int64
	require.NoError(t, db.Model(&lifecycleEdge{}).Where(models.ActiveEdgePredicate).Count(&active).Error)
	assert.Zero(t, active, "every edge is ended, superseded, or suppressed")
}

func TestTemporalEdgeLifecycle_PostgresConcurrentSupersede(t *testing.T) {
	db := newLifecycleEdgeDB(t)
	transactor := models.NewTransactor(db)
	ctx := t.Context()
	owner := lifecycleOwnerID(models.NewID())
	scope := models.EdgeWhere("owner_id = ?", owner)

	edge := newLifecycleEdge(owner, "original", 0.9)
	require.NoError(t, db.Create(edge).Error)

	replacementA := newLifecycleEdge(owner, "from A", 0.9)
	replacementB := newLifecycleEdge(owner, "from B", 0.9)

	releaseA := make(chan struct{})
	release := sync.OnceFunc(func() { close(releaseA) })
	t.Cleanup(release)

	superseded := make(chan struct{}, 1)
	doneA := make(chan error, 1)

	go func() {
		doneA <- transactor.WithinTx(ctx, func(ctx context.Context) error {
			if err := models.SupersedeEdge(ctx, edge.ID, replacementA, scope); err != nil {
				return err
			}

			superseded <- struct{}{}
			<-releaseA

			return nil
		})
	}()

	receive(t, superseded, doneA, "A superseding the edge")

	pidB := make(chan int64, 1)
	doneB := make(chan error, 1)

	go func() {
		doneB <- transactor.WithinTx(ctx, func(ctx context.Context) error {
			pid, err := backendPID(ctx)
			if err != nil {
				return err
			}

			pidB <- pid

			return models.SupersedeEdge(ctx, edge.ID, replacementB, scope)
		})
	}()

	waitUntilBlocked(t, db, receive(t, pidB, doneB, "B's backend pid"))
	release()

	require.NoError(t, <-doneA, "the first supersede succeeds")
	require.ErrorIs(t, <-doneB, models.ErrNotFound, "the second re-checks the predicate after waiting and matches nothing")

	var rows []lifecycleEdge
	require.NoError(t, db.Order("id").Find(&rows).Error)
	require.Len(t, rows, 2, "the edge and A's replacement only")

	var active []lifecycleEdge
	require.NoError(t, db.Where(models.ActiveEdgePredicate).Find(&active).Error)
	require.Len(t, active, 1)
	assert.Equal(t, replacementA.ID, active[0].ID)
}

func TestTemporalEdgeLifecycle_PostgresEndEdgeCopiesCommittedValues(t *testing.T) {
	db := newLifecycleEdgeDB(t)
	transactor := models.NewTransactor(db)
	ctx := t.Context()
	owner := lifecycleOwnerID(models.NewID())

	edge := newLifecycleEdge(owner, "original", 0.9)
	require.NoError(t, db.Create(edge).Error)

	releaseB := make(chan struct{})
	release := sync.OnceFunc(func() { close(releaseB) })
	t.Cleanup(release)

	// B rewrites the confidence in place by id, as a job that recomputes
	// confidence might, and holds its row lock until released.
	rewritten := make(chan struct{}, 1)
	doneB := make(chan error, 1)

	go func() {
		doneB <- transactor.WithinTx(ctx, func(ctx context.Context) error {
			err := models.DBFrom(ctx, nil).
				Exec("UPDATE lifecycle_edges SET confidence = 0.5 WHERE id = ?", string(edge.ID)).Error
			if err != nil {
				return err
			}

			rewritten <- struct{}{}
			<-releaseB

			return nil
		})
	}()

	receive(t, rewritten, doneB, "B rewriting the confidence")

	pidA := make(chan int64, 1)
	doneA := make(chan error, 1)

	var closed *lifecycleEdge

	go func() {
		doneA <- transactor.WithinTx(ctx, func(ctx context.Context) error {
			pid, err := backendPID(ctx)
			if err != nil {
				return err
			}

			pidA <- pid

			closed, err = models.EndEdge[lifecycleEdge](ctx, edge.ID, models.EdgeWhere("owner_id = ?", owner))

			return err
		})
	}()

	waitUntilBlocked(t, db, receive(t, pidA, doneA, "A's backend pid"))
	release()

	require.NoError(t, <-doneB)
	require.NoError(t, <-doneA)
	require.NotNil(t, closed)
	require.NotNil(t, closed.Confidence)
	assert.InDelta(t, 0.5, *closed.Confidence, 1e-9, "the returned copy carries the committed confidence")

	stored := storedLifecycleEdge(t, db, closed.ID)
	require.NotNil(t, stored.Confidence)
	assert.InDelta(t, 0.5, *stored.Confidence, 1e-9, "the stored copy carries the committed confidence")

	original := storedLifecycleEdge(t, db, edge.ID)
	require.NotNil(t, original.SupersededByID)
	assert.Equal(t, closed.ID, *original.SupersededByID)
}
