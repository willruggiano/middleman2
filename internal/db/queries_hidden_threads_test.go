package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHiddenReviewThreadsUpsertAndDelete(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	d := openTestDB(t)
	ctx := context.Background()
	mrID := seedAIQuestionTestMR(t, d)

	// No rows initially.
	rows, err := d.ListHiddenReviewThreads(ctx, mrID)
	require.NoError(err)
	assert.Empty(rows)

	t0 := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	require.NoError(d.UpsertHiddenReviewThread(ctx, mrID, 42, t0))

	rows, err = d.ListHiddenReviewThreads(ctx, mrID)
	require.NoError(err)
	require.Len(rows, 1)
	assert.Equal(int64(42), rows[0].RootPlatformCommentID)
	assert.True(rows[0].HiddenAt.Equal(t0))

	// Re-hide overwrites hidden_at.
	t1 := t0.Add(time.Hour)
	require.NoError(d.UpsertHiddenReviewThread(ctx, mrID, 42, t1))
	rows, err = d.ListHiddenReviewThreads(ctx, mrID)
	require.NoError(err)
	require.Len(rows, 1)
	assert.True(rows[0].HiddenAt.Equal(t1))

	// Delete is idempotent.
	require.NoError(d.DeleteHiddenReviewThread(ctx, mrID, 42))
	require.NoError(d.DeleteHiddenReviewThread(ctx, mrID, 42))
	rows, err = d.ListHiddenReviewThreads(ctx, mrID)
	require.NoError(err)
	assert.Empty(rows)
}

func TestHiddenReviewThreadsCascadeDelete(t *testing.T) {
	require := require.New(t)
	d := openTestDB(t)
	ctx := context.Background()
	mrID := seedAIQuestionTestMR(t, d)

	require.NoError(d.UpsertHiddenReviewThread(
		ctx, mrID, 99, time.Now().UTC().Truncate(time.Second),
	))

	_, err := d.rw.ExecContext(ctx,
		`DELETE FROM middleman_merge_requests WHERE id = ?`, mrID,
	)
	require.NoError(err)

	rows, err := d.ListHiddenReviewThreads(ctx, mrID)
	require.NoError(err)
	require.Empty(rows, "rows should cascade with the MR")
}
