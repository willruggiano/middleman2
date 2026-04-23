package server

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/wesm/middleman/internal/db"
)

// --- Global "Claude sessions" view ---

// aiSessionThread / aiSessionBrief are the wire shapes for the
// /ai/sessions endpoint. Flattened so the UI can render a single
// list without another lookup per row.
type aiSessionThread struct {
	ID                  int64  `json:"id"`
	MRID                int64  `json:"mr_id"`
	RepoOwner           string `json:"repo_owner"`
	RepoName            string `json:"repo_name"`
	PlatformHost        string `json:"platform_host,omitempty"`
	MRNumber            int    `json:"mr_number"`
	MRTitle             string `json:"mr_title"`
	Path                string `json:"path"`
	AnchorSide          string `json:"anchor_side"`
	AnchorLine          int    `json:"anchor_line"`
	CreatedAt           string `json:"created_at"`
	LatestQuestionStatus string `json:"latest_question_status,omitempty"`
	OpenQuestionCount   int    `json:"open_question_count"`
	HasWorktree         bool   `json:"has_worktree"`
}

type aiSessionBrief struct {
	ID           int64  `json:"id"`
	MRID         int64  `json:"mr_id"`
	RepoOwner    string `json:"repo_owner"`
	RepoName     string `json:"repo_name"`
	PlatformHost string `json:"platform_host,omitempty"`
	MRNumber     int    `json:"mr_number"`
	MRTitle      string `json:"mr_title"`
	Status       string `json:"status"`
	Depth        string `json:"depth"`
	CreatedAt    string `json:"created_at"`
	StartedAt    string `json:"started_at,omitempty"`
}

type aiSessionsResponse struct {
	Threads []aiSessionThread `json:"threads"`
	Briefs  []aiSessionBrief  `json:"briefs"`
}

type getAISessionsOutput struct {
	Body aiSessionsResponse
}

func (s *Server) getAISessions(ctx context.Context, _ *struct{}) (*getAISessionsOutput, error) {
	threads, err := s.db.ListActiveAIThreads(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("list active threads: " + err.Error())
	}
	briefs, err := s.db.ListInFlightAIBriefs(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("list in-flight briefs: " + err.Error())
	}

	resp := aiSessionsResponse{
		Threads: make([]aiSessionThread, 0, len(threads)),
		Briefs:  make([]aiSessionBrief, 0, len(briefs)),
	}
	for _, t := range threads {
		row := aiSessionThread{
			ID:                  t.ID,
			MRID:                t.MergeRequestID,
			RepoOwner:           t.RepoOwner,
			RepoName:            t.RepoName,
			PlatformHost:        t.PlatformHost,
			MRNumber:            t.MRNumber,
			MRTitle:             t.MRTitle,
			Path:                t.Path,
			AnchorSide:          t.AnchorSide,
			AnchorLine:          t.AnchorLine,
			CreatedAt:           t.CreatedAt.UTC().Format(time.RFC3339),
			LatestQuestionStatus: t.LatestQuestionStatus,
			OpenQuestionCount:   t.OpenQuestionCount,
			HasWorktree:         t.WorktreePath != nil && *t.WorktreePath != "",
		}
		resp.Threads = append(resp.Threads, row)
	}
	for _, b := range briefs {
		row := aiSessionBrief{
			ID:           b.ID,
			MRID:         b.MergeRequestID,
			RepoOwner:    b.RepoOwner,
			RepoName:     b.RepoName,
			PlatformHost: b.PlatformHost,
			MRNumber:     b.MRNumber,
			MRTitle:      b.MRTitle,
			Status:       b.Status,
			Depth:        b.Depth,
			CreatedAt:    b.CreatedAt.UTC().Format(time.RFC3339),
		}
		if b.StartedAt != nil {
			row.StartedAt = b.StartedAt.UTC().Format(time.RFC3339)
		}
		resp.Briefs = append(resp.Briefs, row)
	}
	return &getAISessionsOutput{Body: resp}, nil
}

// avoid unused import if db package helpers are not referenced directly.
var _ = db.AIThread{}
