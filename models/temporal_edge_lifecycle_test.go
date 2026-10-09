package models_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mrz1836/go-foundation/models"
)

// ownedEdgeID is the typed ID of ownedEdge.
type ownedEdgeID string

// edgeOwnerID is the typed ID of the party an ownedEdge belongs to.
type edgeOwnerID string

// ownedEdge is an edge model scoped to an owner: the shape the lifecycle
// helpers' conditions exist for.
type ownedEdge struct {
	models.TemporalEdge[ownedEdgeID]

	OwnerID edgeOwnerID `gorm:"type:uuid;not null;index"`
	Label   string
}

// lifecycleCall runs one lifecycle helper against target with conds.
type lifecycleCall struct {
	name string
	run  func(ctx context.Context, target ownedEdge, conds ...models.EdgeCondition) error
}

// lifecycleCalls returns one lifecycleCall per helper. The supersede call's
// replacement belongs to the target's owner.
func lifecycleCalls() []lifecycleCall {
	return []lifecycleCall{
		{
			name: "SupersedeEdge",
			run: func(ctx context.Context, target ownedEdge, conds ...models.EdgeCondition) error {
				replacement := newOwnedEdge(target.OwnerID, "replacement")

				return models.SupersedeEdge(ctx, target.ID, replacement, conds...)
			},
		},
		{
			name: "EndEdge",
			run: func(ctx context.Context, target ownedEdge, conds ...models.EdgeCondition) error {
				_, err := models.EndEdge[ownedEdge](ctx, target.ID, conds...)

				return err
			},
		},
		{
			// Ends one hour after the edge's start: inside its interval, and never
			// after the clock's now.
			name: "EndEdgeAt",
			run: func(ctx context.Context, target ownedEdge, conds ...models.EdgeCondition) error {
				_, err := models.EndEdgeAt[ownedEdge](ctx, target.ID, target.ValidFrom.Add(time.Hour), conds...)

				return err
			},
		},
		{
			name: "EndEdgeAtWith",
			run: func(ctx context.Context, target ownedEdge, conds ...models.EdgeCondition) error {
				finish := func(closed *ownedEdge) { closed.Label = "closed" }
				_, err := models.EndEdgeAtWith[ownedEdge](ctx, target.ID, target.ValidFrom.Add(time.Hour), finish, conds...)

				return err
			},
		},
		{
			name: "SuppressEdge",
			run: func(ctx context.Context, target ownedEdge, conds ...models.EdgeCondition) error {
				return models.SuppressEdge[ownedEdge](ctx, target.ID, conds...)
			},
		},
	}
}

// currentLifecycleCalls returns one lifecycleCall per current-row helper. They
// can't join lifecycleCalls: its inactive-edge test requires that no helper
// touch an ended edge, which is what these helpers exist to do. The supersede
// call's replacement is an open edge of the target's owner.
func currentLifecycleCalls() []lifecycleCall {
	return []lifecycleCall{
		{
			name: "SupersedeCurrentEdge",
			run: func(ctx context.Context, target ownedEdge, conds ...models.EdgeCondition) error {
				replacement := newOwnedEdge(target.OwnerID, "reopened")

				return models.SupersedeCurrentEdge(ctx, target.ID, replacement, conds...)
			},
		},
		{
			name: "SuppressCurrentEdge",
			run: func(ctx context.Context, target ownedEdge, conds ...models.EdgeCondition) error {
				return models.SuppressCurrentEdge[ownedEdge](ctx, target.ID, conds...)
			},
		},
	}
}

// newLifecycleDB opens an in-memory SQLite database holding owned_edges, with a
// partial unique index that allows one active edge per owner, as an edge table
// in production would have. A supersede that inserted before it updated would
// violate that index.
func newLifecycleDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Every pooled connection to ":memory:" is its own empty database, so the
	// pool keeps exactly one.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&ownedEdge{}))
	require.NoError(t, db.Exec(
		"CREATE UNIQUE INDEX owned_edges_one_active ON owned_edges (owner_id) WHERE "+models.ActiveEdgePredicate,
	).Error)

	return db
}

// newOwnerID mints a fresh owner ID.
func newOwnerID() edgeOwnerID {
	return edgeOwnerID(models.NewID())
}

// newOwnedEdge returns an unsaved active edge for owner with every provenance
// field set, so a test can tell whether a helper kept or changed each one.
func newOwnedEdge(owner edgeOwnerID, label string) *ownedEdge {
	eventTime := time.Date(2025, 12, 31, 12, 0, 0, 0, time.UTC)
	confidence := 0.75

	return &ownedEdge{
		TemporalEdge: models.TemporalEdge[ownedEdgeID]{
			ValidFrom:           time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			RecordedAt:          time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC),
			EventTime:           &eventTime,
			Source:              "county-registry",
			ByteHash:            "9f2c4e",
			RuleVersion:         "rule-7",
			PipelineVersionHash: "pipeline-3",
			Confidence:          &confidence,
			VerificationStatus:  models.VerificationStatusVerified,
			CreatedAt:           time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC),
		},
		OwnerID: owner,
		Label:   label,
	}
}

// createEdge stores edge and returns it.
func createEdge(t *testing.T, db *gorm.DB, edge *ownedEdge) ownedEdge {
	t.Helper()

	require.NoError(t, db.Create(edge).Error)

	return *edge
}

// edgeByID reads one stored edge.
func edgeByID(t *testing.T, db *gorm.DB, id ownedEdgeID) ownedEdge {
	t.Helper()

	var edge ownedEdge
	require.NoError(t, db.Take(&edge, "id = ?", string(id)).Error)

	return edge
}

// edgeRows returns every stored edge, every column, ordered by id, so a test
// can compare the whole table before and after a call.
func edgeRows(t *testing.T, db *gorm.DB) []ownedEdge {
	t.Helper()

	var rows []ownedEdge
	require.NoError(t, db.Order("id").Find(&rows).Error)

	return rows
}

// activeEdges returns the active edges of owner.
func activeEdges(t *testing.T, db *gorm.DB, owner edgeOwnerID) []ownedEdge {
	t.Helper()

	var rows []ownedEdge
	require.NoError(t, db.Where("owner_id = ?", owner).Where(models.ActiveEdgePredicate).Find(&rows).Error)

	return rows
}

// callCommitted runs call inside a transaction that commits whatever call
// returns, and hands back call's error. Anything a failing helper wrote would
// stay in the table, so comparing the rows afterwards proves the helper itself
// wrote nothing, with no rollback to hide it.
func callCommitted(t *testing.T, ctx context.Context, db *gorm.DB, call func(ctx context.Context) error) error {
	t.Helper()

	var callErr error

	require.NoError(t, models.NewTransactor(db).WithinTx(ctx, func(txCtx context.Context) error {
		callErr = call(txCtx)

		return nil
	}))

	return callErr
}

// utcEdge returns edge with every time in UTC, so a value built in Go compares
// equal to the same value read back from the database.
func utcEdge(edge ownedEdge) ownedEdge {
	e := edge.Edge()
	e.ValidFrom = e.ValidFrom.UTC()
	e.RecordedAt = e.RecordedAt.UTC()
	e.CreatedAt = e.CreatedAt.UTC()
	e.ValidTo = utcTime(e.ValidTo)
	e.EventTime = utcTime(e.EventTime)
	e.SuppressedAt = utcTime(e.SuppressedAt)

	return edge
}

// utcTime returns a copy of *ts in UTC, or nil.
func utcTime(ts *time.Time) *time.Time {
	if ts == nil {
		return nil
	}

	utc := ts.UTC()

	return &utc
}

// requireMisuseError asserts err reports a programming error: it wraps
// ErrValidation but is not a *ValidationError, which callers treat as a
// client's field error.
func requireMisuseError(t *testing.T, err error) {
	t.Helper()

	require.ErrorIs(t, err, models.ErrValidation)

	var fieldErr *models.ValidationError
	require.NotErrorAs(t, err, &fieldErr)
}

// endedEdge stores an active edge for owner, ends it at 2026-02-01 with
// EndEdgeAt, and returns the original and its closed copy, as stored.
func endedEdge(t *testing.T, db *gorm.DB, owner edgeOwnerID) (original, closed ownedEdge) {
	t.Helper()

	stored := createEdge(t, db, newOwnedEdge(owner, "original"))
	endedAt := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	var copied *ownedEdge

	require.NoError(t, callCommitted(t, t.Context(), db, func(ctx context.Context) error {
		var endErr error
		copied, endErr = models.EndEdgeAt[ownedEdge](ctx, stored.ID, endedAt, models.EdgeWhere("owner_id = ?", owner))

		return endErr
	}))

	return edgeByID(t, db, stored.ID), edgeByID(t, db, copied.ID)
}

// currentTarget names a row a current-row helper acts on, and stores it.
type currentTarget struct {
	name  string
	store func(t *testing.T, db *gorm.DB, owner edgeOwnerID) ownedEdge
}

// currentTargets returns the two rows a current-row helper acts on: an ended
// edge's closed copy, and an active edge.
func currentTargets() []currentTarget {
	return []currentTarget{
		{name: "an ended edge", store: func(t *testing.T, db *gorm.DB, owner edgeOwnerID) ownedEdge {
			t.Helper()

			_, closed := endedEdge(t, db, owner)

			return closed
		}},
		{name: "an active edge", store: func(t *testing.T, db *gorm.DB, owner edgeOwnerID) ownedEdge {
			t.Helper()

			return createEdge(t, db, newOwnedEdge(owner, "active"))
		}},
	}
}

func TestEdgeLifecycle_ScopeMismatchWritesNothing(t *testing.T) {
	t.Parallel()

	for _, call := range lifecycleCalls() {
		t.Run(call.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			target := createEdge(t, db, newOwnedEdge(newOwnerID(), "first"))
			before := edgeRows(t, db)

			err := callCommitted(t, t.Context(), db, func(ctx context.Context) error {
				return call.run(ctx, target, models.EdgeWhere("owner_id = ?", newOwnerID()))
			})

			require.ErrorIs(t, err, models.ErrNotFound)
			assert.Equal(t, before, edgeRows(t, db), "a helper scoped to another owner must write nothing")
		})
	}
}

func TestEdgeLifecycle_InactiveEdgeIsNeverTouched(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	states := []struct {
		name       string
		deactivate func(edge *ownedEdge)
	}{
		{name: "ended", deactivate: func(edge *ownedEdge) { edge.ValidTo = &at }},
		{name: "superseded", deactivate: func(edge *ownedEdge) {
			successor := ownedEdgeID(models.NewID())
			edge.SupersededByID = &successor
		}},
		{name: "suppressed", deactivate: func(edge *ownedEdge) { edge.SuppressedAt = &at }},
	}

	for _, state := range states {
		for _, call := range lifecycleCalls() {
			t.Run(state.name+"/"+call.name, func(t *testing.T) {
				t.Parallel()

				db := newLifecycleDB(t)
				owner := newOwnerID()
				seed := newOwnedEdge(owner, "inactive")
				state.deactivate(seed)
				target := createEdge(t, db, seed)
				before := edgeRows(t, db)

				err := callCommitted(t, t.Context(), db, func(ctx context.Context) error {
					return call.run(ctx, target, models.EdgeWhere("owner_id = ?", owner))
				})

				require.ErrorIs(t, err, models.ErrNotFound)
				assert.Equal(t, before, edgeRows(t, db), "an inactive edge must never change")
			})
		}
	}
}

func TestEdgeLifecycle_ConditionCannotWidenWrite(t *testing.T) {
	t.Parallel()

	for _, call := range lifecycleCalls() {
		t.Run(call.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			first := createEdge(t, db, newOwnedEdge(newOwnerID(), "first"))
			second := createEdge(t, db, newOwnedEdge(newOwnerID(), "second"))
			secondBefore := edgeByID(t, db, second.ID)

			// GORM leaves this fragment unparenthesized (its OR follows a
			// newline, not a space), so unwrapped it would OR with the whole
			// WHERE and match every row. Wrapped, it is true for the target
			// alone, and the call goes through on that one edge.
			err := callCommitted(t, t.Context(), db, func(ctx context.Context) error {
				return call.run(ctx, first, models.EdgeWhere("label = ?\nOR 1 = 1", "no-match"))
			})
			require.NoError(t, err)

			assert.Equal(t, secondBefore, edgeByID(t, db, second.ID), "the other owner's edge must not change")

			firstAfter := edgeByID(t, db, first.ID)
			if firstAfter.SupersededByID != nil {
				successor := edgeByID(t, db, *firstAfter.SupersededByID)
				assert.Equal(t, first.OwnerID, successor.OwnerID, "the successor must be built from the target")
			}
		})
	}
}

func TestEdgeLifecycle_RejectsUnbalancedCondition(t *testing.T) {
	t.Parallel()

	conditions := []struct {
		name  string
		query string
	}{
		{name: "closes the parentheses early", query: "label = ?)\nOR\n(1 = 1"},
		{name: "leaves a parenthesis open", query: "(label = ?"},
		{name: "line comment", query: "label = ? -- trailing"},
		{name: "block comment", query: "label = ? /* trailing */"},
		{name: "statement separator", query: "label = ?; SELECT 1"},
	}

	for _, cond := range conditions {
		for _, call := range lifecycleCalls() {
			t.Run(cond.name+"/"+call.name, func(t *testing.T) {
				t.Parallel()

				db := newLifecycleDB(t)
				first := createEdge(t, db, newOwnedEdge(newOwnerID(), "first"))
				createEdge(t, db, newOwnedEdge(newOwnerID(), "second"))
				before := edgeRows(t, db)

				err := callCommitted(t, t.Context(), db, func(ctx context.Context) error {
					return call.run(ctx, first, models.EdgeWhere(cond.query, "no-match"))
				})

				requireMisuseError(t, err)
				assert.Equal(t, before, edgeRows(t, db), "a refused condition must write nothing")
			})
		}
	}
}

func TestSupersedeEdge_LeavesExactlyOneActiveEdge(t *testing.T) {
	t.Parallel()

	db := newLifecycleDB(t)
	owner := newOwnerID()
	original := createEdge(t, db, newOwnedEdge(owner, "original"))
	originalBefore := edgeByID(t, db, original.ID)

	replacement := newOwnedEdge(owner, "corrected")
	require.Empty(t, replacement.ID)

	// The partial unique index allows one active edge per owner, so this call
	// succeeds only when the original is superseded before the replacement is
	// inserted.
	require.NoError(t, callCommitted(t, t.Context(), db, func(ctx context.Context) error {
		return models.SupersedeEdge(ctx, original.ID, replacement, models.EdgeWhere("owner_id = ?", owner))
	}))

	parsed, err := uuid.Parse(string(replacement.ID))
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(7), parsed.Version())

	originalAfter := edgeByID(t, db, original.ID)
	require.NotNil(t, originalAfter.SupersededByID)
	assert.Equal(t, replacement.ID, *originalAfter.SupersededByID)

	originalAfter.SupersededByID = nil
	assert.Equal(t, originalBefore, originalAfter, "only superseded_by_id may change on the original")

	active := activeEdges(t, db, owner)
	require.Len(t, active, 1)
	assert.Equal(t, replacement.ID, active[0].ID)
	assert.Equal(t, "corrected", active[0].Label)
}

func TestEndEdge_ClosedCopyKeepsValidFromAndProvenance(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	cases := []struct {
		name      string
		validFrom time.Time
	}{
		{name: "valid from before now", validFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{name: "valid from after now", validFrom: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			owner := newOwnerID()
			seed := newOwnedEdge(owner, "original")
			seed.ValidFrom = tc.validFrom
			original := createEdge(t, db, seed)
			originalBefore := edgeByID(t, db, original.ID)

			ctx := models.WithClock(t.Context(), models.NewFixedClock(now))

			var closed *ownedEdge

			require.NoError(t, callCommitted(t, ctx, db, func(ctx context.Context) error {
				var endErr error
				closed, endErr = models.EndEdge[ownedEdge](ctx, original.ID, models.EdgeWhere("owner_id = ?", owner))

				return endErr
			}))
			require.NotNil(t, closed)

			parsed, err := uuid.Parse(string(closed.ID))
			require.NoError(t, err)
			assert.Equal(t, uuid.Version(7), parsed.Version())
			assert.NotEqual(t, original.ID, closed.ID)

			// The copy is the original, field by field, with a new ID and
			// closed at the clock's now.
			want := originalBefore
			want.ID = closed.ID
			want.ValidTo = &now
			want.RecordedAt = now
			want.CreatedAt = now
			assert.Equal(t, utcEdge(want), utcEdge(*closed))
			assert.Equal(t, tc.validFrom, closed.ValidFrom.UTC(), "the copy keeps the original's valid_from")
			assert.Equal(t, utcEdge(*closed), utcEdge(edgeByID(t, db, closed.ID)), "the returned copy is the stored row")

			originalAfter := edgeByID(t, db, original.ID)
			assert.Nil(t, originalAfter.ValidTo, "the original's valid_to is never written")
			require.NotNil(t, originalAfter.SupersededByID)
			assert.Equal(t, closed.ID, *originalAfter.SupersededByID)

			originalAfter.SupersededByID = nil
			assert.Equal(t, originalBefore, originalAfter, "only superseded_by_id may change on the original")

			assert.Empty(t, activeEdges(t, db, owner), "no active edge remains for the owner")
		})
	}
}

func TestEndEdgeAt_ClosedCopyEndsAtTheGivenTime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	cases := []struct {
		name    string
		validTo time.Time
	}{
		{
			name:    "between valid_from and now, in another zone",
			validTo: time.Date(2026, 2, 14, 13, 45, 30, 0, time.FixedZone("UTC-5", -5*60*60)),
		},
		{name: "at valid_from", validTo: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{name: "at the clock's now", validTo: now},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			owner := newOwnerID()
			original := createEdge(t, db, newOwnedEdge(owner, "original"))
			originalBefore := edgeByID(t, db, original.ID)

			ctx := models.WithClock(t.Context(), models.NewFixedClock(now))

			var closed *ownedEdge

			require.NoError(t, callCommitted(t, ctx, db, func(ctx context.Context) error {
				var endErr error
				closed, endErr = models.EndEdgeAt[ownedEdge](ctx, original.ID, tc.validTo, models.EdgeWhere("owner_id = ?", owner))

				return endErr
			}))
			require.NotNil(t, closed)

			parsed, err := uuid.Parse(string(closed.ID))
			require.NoError(t, err)
			assert.Equal(t, uuid.Version(7), parsed.Version())
			assert.NotEqual(t, original.ID, closed.ID)

			// The copy is the original, field by field, with a new ID, ended at
			// the given time, and stamped at the clock's now.
			want := originalBefore
			want.ID = closed.ID
			want.ValidTo = &tc.validTo
			want.RecordedAt = now
			want.CreatedAt = now
			assert.Equal(t, utcEdge(want), utcEdge(*closed))

			stored := edgeByID(t, db, closed.ID)
			require.NotNil(t, closed.ValidTo)
			require.NotNil(t, stored.ValidTo)
			assert.True(t, closed.ValidTo.Equal(tc.validTo), "the returned copy ends at the given time")
			assert.True(t, stored.ValidTo.Equal(tc.validTo), "the stored copy ends at the given time")
			assert.Equal(t, utcEdge(*closed), utcEdge(stored), "the returned copy is the stored row")

			originalAfter := edgeByID(t, db, original.ID)
			assert.Nil(t, originalAfter.ValidTo, "the original's valid_to is never written")
			require.NotNil(t, originalAfter.SupersededByID)
			assert.Equal(t, closed.ID, *originalAfter.SupersededByID)

			originalAfter.SupersededByID = nil
			assert.Equal(t, originalBefore, originalAfter, "only superseded_by_id may change on the original")

			assert.Empty(t, activeEdges(t, db, owner), "no active edge remains for the owner")
		})
	}
}

func TestEndEdgeAt_RefusesZeroOrFutureTimeBeforeAnySQL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	cases := []struct {
		name    string
		validTo time.Time
	}{
		{name: "zero", validTo: time.Time{}},
		{name: "a nanosecond after now", validTo: now.Add(time.Nanosecond)},
		{name: "a day after now", validTo: now.AddDate(0, 0, 1)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			owner := newOwnerID()
			stored := createEdge(t, db, newOwnedEdge(owner, "original"))
			before := edgeRows(t, db)
			ctx := models.WithClock(t.Context(), models.NewFixedClock(now))
			scope := models.EdgeWhere("owner_id = ?", owner)

			err := callCommitted(t, ctx, db, func(ctx context.Context) error {
				_, endErr := models.EndEdgeAt[ownedEdge](ctx, stored.ID, tc.validTo, scope)

				return endErr
			})
			requireMisuseError(t, err)
			assert.Equal(t, before, edgeRows(t, db), "a refused end must write nothing")

			// An id that matches no row gets the same refusal, never
			// ErrNotFound: the time is checked before the read.
			err = callCommitted(t, ctx, db, func(ctx context.Context) error {
				_, endErr := models.EndEdgeAt[ownedEdge](ctx, ownedEdgeID(models.NewID()), tc.validTo, scope)

				return endErr
			})
			requireMisuseError(t, err)
			require.NotErrorIs(t, err, models.ErrNotFound)
			assert.Equal(t, before, edgeRows(t, db), "a refused end must write nothing")
		})
	}
}

func TestEndEdgeAt_RefusesTimeBeforeValidFrom(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	cases := []struct {
		name      string
		validFrom time.Time
		validTo   time.Time
	}{
		{
			name:      "a nanosecond before valid_from",
			validFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			validTo:   time.Date(2025, 12, 31, 23, 59, 59, 999999999, time.UTC),
		},
		{
			name:      "a year before valid_from",
			validFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			validTo:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			// EndEdge ends this edge at now; EndEdgeAt refuses the same end.
			name:      "valid_from after the clock's now",
			validFrom: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			validTo:   now,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			owner := newOwnerID()
			seed := newOwnedEdge(owner, "original")
			seed.ValidFrom = tc.validFrom
			original := createEdge(t, db, seed)
			before := edgeRows(t, db)
			ctx := models.WithClock(t.Context(), models.NewFixedClock(now))

			err := callCommitted(t, ctx, db, func(ctx context.Context) error {
				_, endErr := models.EndEdgeAt[ownedEdge](ctx, original.ID, tc.validTo, models.EdgeWhere("owner_id = ?", owner))

				return endErr
			})

			requireMisuseError(t, err)
			assert.Equal(t, before, edgeRows(t, db), "a refused end must write nothing")

			active := activeEdges(t, db, owner)
			require.Len(t, active, 1)
			assert.Equal(t, original.ID, active[0].ID, "the edge stays active")
		})
	}
}

func TestEndEdgeAtWith_FinishSetsOnlyTheCopysOwnColumns(t *testing.T) {
	t.Parallel()

	db := newLifecycleDB(t)
	owner := newOwnerID()
	original := createEdge(t, db, newOwnedEdge(owner, "original"))

	now := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	validTo := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	ctx := models.WithClock(t.Context(), models.NewFixedClock(now))

	// finish sets the copy's own column, and also tries to set the columns the
	// helper owns.
	stale := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	presetID := ownedEdgeID(models.NewID())
	finish := func(closed *ownedEdge) {
		closed.Label = "closed"
		closed.ID = presetID
		closed.ValidTo = &stale
		closed.RecordedAt = stale
		closed.CreatedAt = stale
	}

	var closed *ownedEdge

	require.NoError(t, callCommitted(t, ctx, db, func(ctx context.Context) error {
		var endErr error
		closed, endErr = models.EndEdgeAtWith[ownedEdge](
			ctx, original.ID, validTo, finish, models.EdgeWhere("owner_id = ?", owner),
		)

		return endErr
	}))
	require.NotNil(t, closed)

	for _, got := range []ownedEdge{*closed, edgeByID(t, db, closed.ID)} {
		assert.Equal(t, "closed", got.Label, "finish sets the copy's own column")
		assert.NotEqual(t, presetID, got.ID, "finish can't set the copy's ID")

		parsed, err := uuid.Parse(string(got.ID))
		require.NoError(t, err)
		assert.Equal(t, uuid.Version(7), parsed.Version())

		require.NotNil(t, got.ValidTo)
		assert.True(t, got.ValidTo.Equal(validTo), "finish can't move the end")
		assert.True(t, got.RecordedAt.Equal(now), "finish can't set recorded_at")
		assert.True(t, got.CreatedAt.Equal(now), "finish can't set created_at")
	}

	assert.Equal(t, "original", edgeByID(t, db, original.ID).Label, "finish never touches the original")
}

func TestEndEdgeAtWith_RefusedCallNeverRunsFinish(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	validTo := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	refusals := []struct {
		name       string
		validTo    time.Time
		otherOwner bool
		wantErr    error
	}{
		{name: "zero time", validTo: time.Time{}, wantErr: models.ErrValidation},
		{name: "future time", validTo: now.Add(time.Hour), wantErr: models.ErrValidation},
		{name: "time before valid_from", validTo: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), wantErr: models.ErrValidation},
		{name: "another owner's scope", validTo: validTo, otherOwner: true, wantErr: models.ErrNotFound},
	}

	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			owner := newOwnerID()
			target := createEdge(t, db, newOwnedEdge(owner, "original"))
			before := edgeRows(t, db)
			ctx := models.WithClock(t.Context(), models.NewFixedClock(now))

			scopeOwner := owner
			if tc.otherOwner {
				scopeOwner = newOwnerID()
			}

			calls := 0
			finish := func(*ownedEdge) { calls++ }

			err := callCommitted(t, ctx, db, func(ctx context.Context) error {
				_, endErr := models.EndEdgeAtWith[ownedEdge](
					ctx, target.ID, tc.validTo, finish, models.EdgeWhere("owner_id = ?", scopeOwner),
				)

				return endErr
			})

			// Every refusal here is a programming error or no match, never a
			// client's field error.
			require.ErrorIs(t, err, tc.wantErr)

			var fieldErr *models.ValidationError
			require.NotErrorAs(t, err, &fieldErr)

			assert.Zero(t, calls, "a refused call never runs finish")
			assert.Equal(t, before, edgeRows(t, db), "a refused call writes nothing")
		})
	}

	// A copy that finish leaves superseded or suppressed is refused before
	// anything is written.
	leftInactive := []struct {
		name   string
		finish func(closed *ownedEdge)
	}{
		{name: "finish leaves the copy superseded", finish: func(closed *ownedEdge) {
			successor := ownedEdgeID(models.NewID())
			closed.SupersededByID = &successor
		}},
		{name: "finish leaves the copy suppressed", finish: func(closed *ownedEdge) { closed.SuppressedAt = &now }},
	}

	for _, tc := range leftInactive {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			owner := newOwnerID()
			target := createEdge(t, db, newOwnedEdge(owner, "original"))
			before := edgeRows(t, db)
			ctx := models.WithClock(t.Context(), models.NewFixedClock(now))

			err := callCommitted(t, ctx, db, func(ctx context.Context) error {
				_, endErr := models.EndEdgeAtWith[ownedEdge](
					ctx, target.ID, validTo, tc.finish, models.EdgeWhere("owner_id = ?", owner),
				)

				return endErr
			})

			requireMisuseError(t, err)
			assert.Equal(t, before, edgeRows(t, db), "a copy left superseded or suppressed is never written")
		})
	}
}

func TestSuppressEdge_SetsOnlySuppressedAt(t *testing.T) {
	t.Parallel()

	db := newLifecycleDB(t)
	owner := newOwnerID()
	original := createEdge(t, db, newOwnedEdge(owner, "original"))
	createEdge(t, db, newOwnedEdge(newOwnerID(), "other"))
	before := edgeRows(t, db)

	now := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	ctx := models.WithClock(t.Context(), models.NewFixedClock(now))

	require.NoError(t, callCommitted(t, ctx, db, func(ctx context.Context) error {
		return models.SuppressEdge[ownedEdge](ctx, original.ID, models.EdgeWhere("owner_id = ?", owner))
	}))

	stored := edgeByID(t, db, original.ID)
	require.NotNil(t, stored.SuppressedAt)
	assert.Equal(t, now, stored.SuppressedAt.UTC())
	assert.Equal(t, models.VerificationStatusVerified, stored.VerificationStatus, "verification_status is not touched")

	after := edgeRows(t, db)
	require.Len(t, after, len(before), "suppressing inserts no row")

	for i := range after {
		if after[i].ID == original.ID {
			after[i].SuppressedAt = nil
		}
	}

	assert.Equal(t, before, after, "only suppressed_at may change")
}

func TestEdgeLifecycle_RequiresTransaction(t *testing.T) {
	t.Parallel()

	for _, call := range lifecycleCalls() {
		t.Run(call.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			owner := newOwnerID()
			target := createEdge(t, db, newOwnedEdge(owner, "first"))
			before := edgeRows(t, db)
			scope := models.EdgeWhere("owner_id = ?", owner)

			err := call.run(t.Context(), target, scope)
			require.ErrorIs(t, err, models.ErrNoTransaction, "a context with no handle")

			err = call.run(models.WithTx(t.Context(), db), target, scope)
			require.ErrorIs(t, err, models.ErrNoTransaction, "a context whose handle is not a transaction")

			assert.Equal(t, before, edgeRows(t, db))
		})
	}
}

func TestEdgeLifecycle_RejectsMalformedID(t *testing.T) {
	t.Parallel()

	for _, call := range lifecycleCalls() {
		t.Run(call.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			owner := newOwnerID()
			createEdge(t, db, newOwnedEdge(owner, "first"))
			before := edgeRows(t, db)

			for _, id := range []ownedEdgeID{"", "not-a-uuid"} {
				target := ownedEdge{TemporalEdge: models.TemporalEdge[ownedEdgeID]{ID: id}, OwnerID: owner}

				err := callCommitted(t, t.Context(), db, func(ctx context.Context) error {
					return call.run(ctx, target, models.EdgeWhere("owner_id = ?", owner))
				})
				require.ErrorIs(t, err, models.ErrInvalidID, "id %q", id)
			}

			assert.Equal(t, before, edgeRows(t, db))
		})
	}
}

func TestEdgeLifecycle_RejectsBlankCondition(t *testing.T) {
	t.Parallel()

	for _, call := range lifecycleCalls() {
		t.Run(call.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			owner := newOwnerID()
			target := createEdge(t, db, newOwnedEdge(owner, "first"))
			before := edgeRows(t, db)

			for _, conds := range [][]models.EdgeCondition{
				{models.EdgeWhere("  ")},
				{models.EdgeCondition{}},
				{models.EdgeWhere("owner_id = ?", owner), models.EdgeWhere("\t\n")},
			} {
				err := callCommitted(t, t.Context(), db, func(ctx context.Context) error {
					return call.run(ctx, target, conds...)
				})
				requireMisuseError(t, err)
			}

			assert.Equal(t, before, edgeRows(t, db))
		})
	}
}

func TestSupersedeEdge_RejectsInvalidReplacement(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name        string
		replacement func(target ownedEdge) *ownedEdge
		wantErr     error
	}{
		{
			name:        "nil replacement",
			replacement: func(ownedEdge) *ownedEdge { return nil },
			wantErr:     models.ErrValidation,
		},
		{
			name: "already superseded",
			replacement: func(target ownedEdge) *ownedEdge {
				r := newOwnedEdge(target.OwnerID, "replacement")
				successor := ownedEdgeID(models.NewID())
				r.SupersededByID = &successor

				return r
			},
			wantErr: models.ErrValidation,
		},
		{
			name: "already suppressed",
			replacement: func(target ownedEdge) *ownedEdge {
				r := newOwnedEdge(target.OwnerID, "replacement")
				r.SuppressedAt = &at

				return r
			},
			wantErr: models.ErrValidation,
		},
		{
			name: "supersedes itself",
			replacement: func(target ownedEdge) *ownedEdge {
				r := newOwnedEdge(target.OwnerID, "replacement")
				r.ID = target.ID

				return r
			},
			wantErr: models.ErrValidation,
		},
		{
			// SQLite does not type-check a uuid column, so only the helper's
			// own check can refuse this ID.
			name: "preset ID is not a UUID",
			replacement: func(target ownedEdge) *ownedEdge {
				r := newOwnedEdge(target.OwnerID, "replacement")
				r.ID = "not-a-uuid"

				return r
			},
			wantErr: models.ErrInvalidID,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			owner := newOwnerID()
			target := createEdge(t, db, newOwnedEdge(owner, "original"))
			before := edgeRows(t, db)

			err := callCommitted(t, t.Context(), db, func(ctx context.Context) error {
				return models.SupersedeEdge(ctx, target.ID, tc.replacement(target), models.EdgeWhere("owner_id = ?", owner))
			})

			if errors.Is(tc.wantErr, models.ErrValidation) {
				requireMisuseError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}

			assert.Equal(t, before, edgeRows(t, db), "the original stays active and nothing is inserted")
		})
	}
}

func TestSupersedeEdge_KeepsPresetReplacementID(t *testing.T) {
	t.Parallel()

	db := newLifecycleDB(t)
	owner := newOwnerID()
	original := createEdge(t, db, newOwnedEdge(owner, "original"))

	preset := ownedEdgeID(models.NewID())
	replacement := newOwnedEdge(owner, "corrected")
	replacement.ID = preset

	require.NoError(t, callCommitted(t, t.Context(), db, func(ctx context.Context) error {
		return models.SupersedeEdge(ctx, original.ID, replacement, models.EdgeWhere("owner_id = ?", owner))
	}))

	assert.Equal(t, preset, replacement.ID)

	originalAfter := edgeByID(t, db, original.ID)
	require.NotNil(t, originalAfter.SupersededByID)
	assert.Equal(t, preset, *originalAfter.SupersededByID)
	assert.Equal(t, "corrected", edgeByID(t, db, preset).Label)
}

func TestSupersedeEdge_FailedInsertRollsBackWithTransaction(t *testing.T) {
	t.Parallel()

	db := newLifecycleDB(t)
	owner := newOwnerID()
	original := createEdge(t, db, newOwnedEdge(owner, "original"))
	other := createEdge(t, db, newOwnedEdge(newOwnerID(), "other"))
	before := edgeRows(t, db)

	// The update succeeds, then the insert collides with another row's ID.
	replacement := newOwnedEdge(owner, "corrected")
	replacement.ID = other.ID

	err := models.NewTransactor(db).WithinTx(t.Context(), func(ctx context.Context) error {
		return models.SupersedeEdge(ctx, original.ID, replacement, models.EdgeWhere("owner_id = ?", owner))
	})
	require.ErrorIs(t, err, models.ErrDuplicateKey)

	assert.Equal(t, before, edgeRows(t, db), "the rollback undoes the update that ran before the failed insert")

	active := activeEdges(t, db, owner)
	require.Len(t, active, 1)
	assert.Equal(t, original.ID, active[0].ID)
}

func TestSupersedeCurrentEdge_ReopensAnEndedEdge(t *testing.T) {
	t.Parallel()

	for _, target := range currentTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			owner := newOwnerID()
			current := target.store(t, db, owner)
			before := edgeRows(t, db)

			// An open copy of the target: for an ended edge, its reopening.
			open := current
			open.ID = ""
			open.ValidTo = nil
			open.Label = "reopened"

			// The partial unique index allows one active edge per owner, so this
			// call succeeds only when the target is superseded before the open
			// copy is inserted.
			require.NoError(t, callCommitted(t, t.Context(), db, func(ctx context.Context) error {
				return models.SupersedeCurrentEdge(ctx, current.ID, &open, models.EdgeWhere("owner_id = ?", owner))
			}))

			currentAfter := edgeByID(t, db, current.ID)
			require.NotNil(t, currentAfter.SupersededByID)
			assert.Equal(t, open.ID, *currentAfter.SupersededByID)

			active := activeEdges(t, db, owner)
			require.Len(t, active, 1)
			assert.Equal(t, open.ID, active[0].ID)
			assert.Equal(t, "reopened", active[0].Label)

			// Every row stored before is unchanged, but for the target's
			// superseded_by_id.
			currentAfter.SupersededByID = nil
			for _, row := range before {
				stored := currentAfter
				if row.ID != current.ID {
					stored = edgeByID(t, db, row.ID)
				}

				assert.Equal(t, row, stored, "only superseded_by_id may change on the target")
			}
		})
	}
}

func TestSuppressCurrentEdge_SuppressesAnEndedEdge(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)

	for _, target := range currentTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			owner := newOwnerID()
			current := target.store(t, db, owner)
			before := edgeRows(t, db)
			ctx := models.WithClock(t.Context(), models.NewFixedClock(now))

			require.NoError(t, callCommitted(t, ctx, db, func(ctx context.Context) error {
				return models.SuppressCurrentEdge[ownedEdge](ctx, current.ID, models.EdgeWhere("owner_id = ?", owner))
			}))

			stored := edgeByID(t, db, current.ID)
			require.NotNil(t, stored.SuppressedAt)
			assert.Equal(t, now, stored.SuppressedAt.UTC())

			after := edgeRows(t, db)
			require.Len(t, after, len(before), "suppressing inserts no row")

			for i := range after {
				if after[i].ID == current.ID {
					after[i].SuppressedAt = nil
				}
			}

			assert.Equal(t, before, after, "only the target's suppressed_at may change")
			assert.Empty(t, activeEdges(t, db, owner), "no active edge remains for the owner")
		})
	}
}

func TestCurrentEdgeLifecycle_NeverMatchesSupersededOrSuppressedRows(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	states := []currentTarget{
		{name: "superseded", store: func(t *testing.T, db *gorm.DB, owner edgeOwnerID) ownedEdge {
			t.Helper()

			// The original of an ended edge: its closed copy supersedes it.
			original, _ := endedEdge(t, db, owner)

			return original
		}},
		{name: "suppressed", store: func(t *testing.T, db *gorm.DB, owner edgeOwnerID) ownedEdge {
			t.Helper()

			seed := newOwnedEdge(owner, "suppressed")
			seed.SuppressedAt = &at

			return createEdge(t, db, seed)
		}},
	}

	for _, state := range states {
		for _, call := range currentLifecycleCalls() {
			t.Run(state.name+"/"+call.name, func(t *testing.T) {
				t.Parallel()

				db := newLifecycleDB(t)
				owner := newOwnerID()
				target := state.store(t, db, owner)
				before := edgeRows(t, db)

				err := callCommitted(t, t.Context(), db, func(ctx context.Context) error {
					return call.run(ctx, target, models.EdgeWhere("owner_id = ?", owner))
				})

				require.ErrorIs(t, err, models.ErrNotFound)
				assert.Equal(t, before, edgeRows(t, db), "a superseded or suppressed row must never change")
			})
		}
	}
}

func TestCurrentEdgeLifecycle_KeepsTheScopeAndTransactionRules(t *testing.T) {
	t.Parallel()

	for _, call := range currentLifecycleCalls() {
		t.Run(call.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			owner := newOwnerID()
			_, closed := endedEdge(t, db, owner)
			before := edgeRows(t, db)
			scope := models.EdgeWhere("owner_id = ?", owner)

			err := callCommitted(t, t.Context(), db, func(ctx context.Context) error {
				return call.run(ctx, closed, models.EdgeWhere("owner_id = ?", newOwnerID()))
			})
			require.ErrorIs(t, err, models.ErrNotFound, "another owner's scope")
			assert.Equal(t, before, edgeRows(t, db))

			err = call.run(t.Context(), closed, scope)
			require.ErrorIs(t, err, models.ErrNoTransaction, "a context with no handle")
			assert.Equal(t, before, edgeRows(t, db))

			err = call.run(models.WithTx(t.Context(), db), closed, scope)
			require.ErrorIs(t, err, models.ErrNoTransaction, "a context whose handle is not a transaction")
			assert.Equal(t, before, edgeRows(t, db))

			for _, id := range []ownedEdgeID{"", "not-a-uuid"} {
				malformed := closed
				malformed.ID = id

				err = callCommitted(t, t.Context(), db, func(ctx context.Context) error {
					return call.run(ctx, malformed, scope)
				})
				require.ErrorIs(t, err, models.ErrInvalidID, "id %q", id)
				assert.Equal(t, before, edgeRows(t, db))
			}

			for _, cond := range []models.EdgeCondition{models.EdgeWhere("  "), {}} {
				err = callCommitted(t, t.Context(), db, func(ctx context.Context) error {
					return call.run(ctx, closed, cond)
				})
				requireMisuseError(t, err)
				assert.Equal(t, before, edgeRows(t, db))
			}
		})
	}
}

func TestCurrentEdgeLifecycle_ConditionCannotWidenWrite(t *testing.T) {
	t.Parallel()

	for _, call := range currentLifecycleCalls() {
		t.Run(call.name, func(t *testing.T) {
			t.Parallel()

			db := newLifecycleDB(t)
			ownerA := newOwnerID()
			_, closedA := endedEdge(t, db, ownerA)
			originalB, closedB := endedEdge(t, db, newOwnerID())

			// GORM leaves this fragment unparenthesized (its OR follows a
			// newline, not a space), so unwrapped it would OR with the whole
			// WHERE and match every current row. Wrapped, it is true for the
			// target alone, and the call goes through on that one row.
			err := callCommitted(t, t.Context(), db, func(ctx context.Context) error {
				return call.run(ctx, closedA, models.EdgeWhere("label = ?\nOR 1 = 1", "no-match"))
			})
			require.NoError(t, err)

			assert.Equal(t, originalB, edgeByID(t, db, originalB.ID), "the other owner's original must not change")
			assert.Equal(t, closedB, edgeByID(t, db, closedB.ID), "the other owner's closed copy must not change")

			closedAfter := edgeByID(t, db, closedA.ID)
			if closedAfter.SupersededByID != nil {
				successor := edgeByID(t, db, *closedAfter.SupersededByID)
				assert.Equal(t, ownerA, successor.OwnerID, "the successor must be built from the target")
			}
		})
	}
}

func TestCurrentEdgeLifecycle_RejectsUnbalancedCondition(t *testing.T) {
	t.Parallel()

	conditions := []struct {
		name  string
		query string
	}{
		{name: "closes the parentheses early", query: "label = ?)\nOR\n(1 = 1"},
		{name: "leaves a parenthesis open", query: "(label = ?"},
		{name: "line comment", query: "label = ? -- trailing"},
		{name: "block comment", query: "label = ? /* trailing */"},
		{name: "statement separator", query: "label = ?; SELECT 1"},
	}

	for _, cond := range conditions {
		for _, call := range currentLifecycleCalls() {
			t.Run(cond.name+"/"+call.name, func(t *testing.T) {
				t.Parallel()

				db := newLifecycleDB(t)
				_, closedA := endedEdge(t, db, newOwnerID())
				endedEdge(t, db, newOwnerID())
				before := edgeRows(t, db)

				err := callCommitted(t, t.Context(), db, func(ctx context.Context) error {
					return call.run(ctx, closedA, models.EdgeWhere(cond.query, "no-match"))
				})

				requireMisuseError(t, err)
				assert.Equal(t, before, edgeRows(t, db), "a refused condition must write nothing")
			})
		}
	}
}

func TestSupersedeCurrentEdge_ReopenIntoAnOccupiedKeyFails(t *testing.T) {
	t.Parallel()

	db := newLifecycleDB(t)
	owner := newOwnerID()
	_, closed := endedEdge(t, db, owner)

	// The closed copy isn't active, so the owner may hold a newer active edge.
	newer := createEdge(t, db, newOwnedEdge(owner, "newer"))
	before := edgeRows(t, db)

	open := closed
	open.ID = ""
	open.ValidTo = nil
	open.Label = "reopened"

	// The update succeeds, then the insert collides with the newer edge under
	// the partial unique index over active rows.
	err := models.NewTransactor(db).WithinTx(t.Context(), func(ctx context.Context) error {
		return models.SupersedeCurrentEdge(ctx, closed.ID, &open, models.EdgeWhere("owner_id = ?", owner))
	})
	require.ErrorIs(t, err, models.ErrDuplicateKey)

	assert.Equal(t, before, edgeRows(t, db), "the rollback undoes the update that ran before the failed insert")
	assert.Nil(t, edgeByID(t, db, closed.ID).SupersededByID, "the closed copy stays current")

	active := activeEdges(t, db, owner)
	require.Len(t, active, 1)
	assert.Equal(t, newer.ID, active[0].ID)
}
