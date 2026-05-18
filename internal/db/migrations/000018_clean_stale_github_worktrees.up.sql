-- An earlier iteration stored worktrees against the GitHub repo
-- entry (when local_path was a field on the GitHub config block).
-- The current schema scopes worktrees to local-only repo entries
-- (platform='local'), but rows from the old shape stay in
-- middleman_worktrees and continue to surface in the API until
-- something cleans them up. Sweep them once here.
--
-- The current sync engine no longer touches GitHub-side repos
-- with the worktree scanner, so this is strictly a one-time
-- cleanup of legacy state; nothing should reintroduce these rows
-- after this migration runs.
DELETE FROM middleman_worktrees
WHERE repo_id IN (
    SELECT id FROM middleman_repos WHERE platform = 'github'
);
