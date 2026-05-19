// Source-discrimination helpers for the review pane.
//
// Worktrees flow through PR-shaped URLs / endpoints with a
// synthetic owner. Components that need to hide GitHub-only
// affordances (Approve / Merge / "View on GitHub") gate on
// isLocalSource(owner); everything else operates uniformly.

export const LOCAL_OWNER = "local";

export function isLocalSource(owner: string | null | undefined): boolean {
  return owner === LOCAL_OWNER;
}
