-- Per-commit Claude analysis. Same shape as middleman_ai_briefs but
-- keyed on (mr_id, commit_sha) so each commit in a PR can hold its
-- own guidance independent of the PR-level brief. Commits are
-- immutable so the analysis cache survives indefinitely; we still
-- key on mr_id because surrounding-commit context (Sequence section)
-- depends on the PR's other commits.
CREATE TABLE IF NOT EXISTS middleman_ai_commit_analyses (
    id                  INTEGER PRIMARY KEY,
    mr_id               INTEGER NOT NULL REFERENCES middleman_merge_requests(id) ON DELETE CASCADE,
    commit_sha          TEXT NOT NULL,
    claude_session_id   TEXT,
    worktree_path       TEXT,
    status              TEXT NOT NULL DEFAULT 'queued',
    content             TEXT NOT NULL DEFAULT '',
    error               TEXT NOT NULL DEFAULT '',
    pid                 INTEGER,
    created_at          DATETIME NOT NULL DEFAULT (datetime('now')),
    started_at          DATETIME,
    completed_at        DATETIME,
    UNIQUE(mr_id, commit_sha)
);

CREATE INDEX IF NOT EXISTS idx_ai_commit_analyses_mr_id
    ON middleman_ai_commit_analyses(mr_id);
