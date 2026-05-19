-- Worktree-scoped interactive Claude sessions. One row per
-- worktree; the row carries the claude_session_id used to --resume
-- subsequent turns. status transitions are:
--
--   active   → an interactive session is alive; reviewer can submit
--              more turns. Initial state on creation.
--   killed   → the user explicitly stopped the session. New input
--              starts a fresh session row.
--   closed   → the worktree was removed or middleman archived the
--              session. Terminal.
--
-- Multiple rows per worktree may exist over time (each "killed"
-- session leaves a row); the live session is the most recent row
-- with status='active'.
CREATE TABLE IF NOT EXISTS middleman_worktree_sessions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    worktree_id       INTEGER NOT NULL REFERENCES middleman_worktrees(id) ON DELETE CASCADE,
    claude_session_id TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'active',
    started_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    last_activity_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_worktree_sessions_worktree_active
    ON middleman_worktree_sessions(worktree_id) WHERE status = 'active';

-- One conversation turn. turn_type is a small discriminator:
--   review_feedback  user, compiled from inline draft comments
--   user_message     user, free-text from the textbox
--   claude_response  claude's reply (may also embed tool calls)
--   state            session-started, session-killed, etc. — markers
--                    so the rendered timeline aligns with system events
--
-- status applies meaningfully to claude_response turns:
--   queued / running / done / failed / cancelled
-- For user turns, status is always 'done'.
--
-- content holds the rendered message (markdown for user turns, the
-- result.text from claude's --output-format=json for claude turns).
-- raw_json keeps claude's full envelope for debugging / future
-- richer rendering (tool calls etc.).
CREATE TABLE IF NOT EXISTS middleman_worktree_session_turns (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id       INTEGER NOT NULL REFERENCES middleman_worktree_sessions(id) ON DELETE CASCADE,
    turn_type        TEXT NOT NULL,
    content          TEXT NOT NULL DEFAULT '',
    raw_json         TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'done',
    error            TEXT NOT NULL DEFAULT '',
    pid              INTEGER,
    metadata_json    TEXT NOT NULL DEFAULT '',
    created_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_worktree_session_turns_session
    ON middleman_worktree_session_turns(session_id, id);
