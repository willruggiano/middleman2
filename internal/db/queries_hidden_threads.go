package db

import (
	"context"
	"time"
)

// HiddenReviewThread is one row in middleman_hidden_review_threads —
// a reviewer's intent to hide a review thread on a specific PR. The
// row is "active" only if no reply in the thread has a created_at
// after HiddenAt; see ActiveHiddenReviewThreadRoots.
type HiddenReviewThread struct {
	MergeRequestID        int64
	RootPlatformCommentID int64
	HiddenAt              time.Time
}

// UpsertHiddenReviewThread records (or re-records) the user's intent
// to hide the given thread. Re-hiding overwrites HiddenAt so a new
// reply that arrived between the two hides doesn't keep the thread
// visible.
func (d *DB) UpsertHiddenReviewThread(
	ctx context.Context,
	mrID, rootPlatformCommentID int64,
	hiddenAt time.Time,
) error {
	_, err := d.rw.ExecContext(ctx,
		`INSERT INTO middleman_hidden_review_threads
		     (merge_request_id, root_platform_comment_id, hidden_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(merge_request_id, root_platform_comment_id) DO UPDATE
		     SET hidden_at = excluded.hidden_at`,
		mrID, rootPlatformCommentID, hiddenAt.UTC(),
	)
	return err
}

// DeleteHiddenReviewThread clears the user's hide for a given thread.
// No-op when the row doesn't exist (DELETE on a missing row returns
// without error).
func (d *DB) DeleteHiddenReviewThread(
	ctx context.Context, mrID, rootPlatformCommentID int64,
) error {
	_, err := d.rw.ExecContext(ctx,
		`DELETE FROM middleman_hidden_review_threads
		  WHERE merge_request_id = ? AND root_platform_comment_id = ?`,
		mrID, rootPlatformCommentID,
	)
	return err
}

// ListHiddenReviewThreads returns every stored row for an MR,
// including rows that may be superseded by a newer reply. Callers
// that need only currently-active hides should pass the result through
// ActiveHiddenReviewThreadRoots.
func (d *DB) ListHiddenReviewThreads(
	ctx context.Context, mrID int64,
) ([]HiddenReviewThread, error) {
	rows, err := d.ro.QueryContext(ctx,
		`SELECT merge_request_id, root_platform_comment_id, hidden_at
		   FROM middleman_hidden_review_threads
		  WHERE merge_request_id = ?`,
		mrID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HiddenReviewThread
	for rows.Next() {
		var h HiddenReviewThread
		if err := rows.Scan(
			&h.MergeRequestID, &h.RootPlatformCommentID, &h.HiddenAt,
		); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
