-- This migration deletes legacy rows from middleman_worktrees that
-- have no place in the post-refactor schema. The deletion is
-- one-way; the down migration is intentionally a no-op.
SELECT 1;
