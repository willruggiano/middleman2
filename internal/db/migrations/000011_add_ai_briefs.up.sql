CREATE TABLE IF NOT EXISTS middleman_ai_briefs (
    id                  INTEGER PRIMARY KEY,
    mr_id               INTEGER NOT NULL REFERENCES middleman_merge_requests(id) ON DELETE CASCADE,
    head_sha            TEXT NOT NULL,
    claude_session_id   TEXT,
    worktree_path       TEXT,
    status              TEXT NOT NULL DEFAULT 'queued', -- 'queued' | 'running' | 'done' | 'failed' | 'cancelled'
    depth               TEXT NOT NULL DEFAULT 'quick',  -- 'quick' | 'deep'
    content             TEXT NOT NULL DEFAULT '',
    error               TEXT NOT NULL DEFAULT '',
    pid                 INTEGER,
    created_at          DATETIME NOT NULL DEFAULT (datetime('now')),
    started_at          DATETIME,
    completed_at        DATETIME,
    UNIQUE(mr_id, head_sha)
);

CREATE INDEX IF NOT EXISTS idx_ai_briefs_mr_id
    ON middleman_ai_briefs(mr_id);
