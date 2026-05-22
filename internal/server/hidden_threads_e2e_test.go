package server

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wesm/middleman/internal/db"
)

// seedReviewComment is one comment to insert via seedReviewComments.
// ID is the GitHub platform comment id; InReplyTo of 0 marks a root.
type seedReviewComment struct {
	ID        int64
	InReplyTo int64
	CreatedAt time.Time
}

func seedReviewComments(t *testing.T, database *db.DB, mrID int64, items []seedReviewComment) {
	t.Helper()
	events := make([]db.MREvent, 0, len(items))
	for _, it := range items {
		id := it.ID
		meta := `{"path":"f.go","line":1,"side":"RIGHT"}`
		if it.InReplyTo != 0 {
			meta = `{"path":"f.go","line":1,"side":"RIGHT","in_reply_to":` +
				strconv.FormatInt(it.InReplyTo, 10) + `}`
		}
		events = append(events, db.MREvent{
			MergeRequestID: mrID,
			PlatformID:     &id,
			EventType:      "review_comment",
			Author:         "reviewer",
			Body:           "comment body",
			CreatedAt:      it.CreatedAt,
			MetadataJSON:   meta,
			DedupeKey:      "review-comment-" + strconv.FormatInt(it.ID, 10),
		})
	}
	require.NoError(t, database.UpsertMREvents(context.Background(), events))
}

func TestPullDetailIncludesEmptyHiddenSetByDefault(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	srv, database := setupTestServer(t)
	seedPR(t, database, "acme", "widget", 1)
	client := setupTestClient(t, srv)

	resp, err := client.HTTP.GetReposByOwnerByNamePullsByNumberWithResponse(
		context.Background(), "acme", "widget", 1,
	)
	require.NoError(err)
	require.Equal(http.StatusOK, resp.StatusCode())
	require.NotNil(resp.JSON200)
	require.NotNil(resp.JSON200.HiddenThreadRootIds, "field should be present and non-nil")
	assert.Empty(*resp.JSON200.HiddenThreadRootIds)
}
