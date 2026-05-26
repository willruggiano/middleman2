# Selective Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Scope the header's Sync button to the repo currently selected in the global dropdown (full sync when nothing is selected), fix the diff toolbar's Refresh button (currently always returns 415), and surface refresh errors visibly.

**Architecture:** Lift the repo slice out of `Syncer.runOnce` into a parameter; add `TriggerRunForRepos([]RepoRef)` that calls `runOnce` with the supplied slice through the existing lifecycle. New `POST /repos/{owner}/{name}/sync` handler validates the repo is tracked then fires `TriggerRunForRepos([oneRepo])`. Frontend Sync button consults `getGlobalRepo()` and routes to the scoped endpoint or the existing `POST /sync`. Diff store's `refresh()` switches from raw `fetch` (which omits `Content-Type` and trips the CSRF middleware's 415) to the typed `apiClient.POST`, with the visible error text driven by the actual server message.

**Tech Stack:** Go 1.x, Huma (OperationID-driven generated client names), modernc.org/sqlite, oapi-codegen Go client, openapi-fetch + openapi-typescript, Svelte 5 (runes), Bun, vitest.

**Spec:** `docs/superpowers/specs/2026-05-22-selective-sync-design.md`

---

## File Map

**New files (Go):**
- `internal/server/huma_routes_sync.go` — input/output types + `triggerSync` (moved) + `syncRepo` handler.
- `internal/server/sync_repo_e2e_test.go` — e2e tests for the new endpoint.

**Modified files (Go):**
- `internal/github/sync.go` — refactor `runOnce` signature, add `TriggerRunForRepos`.
- `internal/github/sync_test.go` — add unit tests for `TriggerRunForRepos`.
- `internal/server/huma_routes.go` — remove `triggerSync` (moves to new file), register new route.

**New files (TypeScript):**
- `packages/ui/src/api/errors.ts` — shared `apiErrorMessage` helper (extracted from `detail.svelte.ts`).
- `packages/ui/src/stores/sync.test.ts` — tests for `triggerSyncForRepo`.
- `packages/ui/src/stores/diff.refresh.test.ts` — focused test for the refresh fix (separate file because `diff.svelte.ts` is large).

**Modified files (TypeScript):**
- `packages/ui/src/stores/sync.svelte.ts` — `triggerSyncForRepo(owner, name)` action.
- `packages/ui/src/stores/detail.svelte.ts` — drop local `apiErrorMessage`, import from `../api/errors.js`.
- `packages/ui/src/stores/diff.svelte.ts` — `DiffStoreOptions` gains `client: MiddlemanClient`; `refresh()` switches to typed call.
- `packages/ui/src/Provider.svelte` — thread `client` into the diff store options.
- `packages/ui/src/components/diff/DiffToolbar.svelte` — render actual error text instead of literal "sync failed", with truncation styling.
- `frontend/src/lib/components/layout/AppHeader.svelte` — `handleSync` routes by `getGlobalRepo()`; button label reflects scope.
- `frontend/src/lib/components/layout/AppHeader.test.ts` — extend with scoped-repo case.

**Modified files (regenerated):**
- `frontend/openapi/openapi.json`
- `internal/apiclient/spec/openapi.json`
- `internal/apiclient/generated/client.gen.go`
- `packages/ui/src/api/generated/schema.ts`
- `packages/ui/src/api/generated/client.ts`

---

## Conventions

- Always commit at the end of each task (CLAUDE.md: "commit every turn").
- Never amend; new commit per task.
- Never bypass pre-commit hooks (no `--no-verify`).
- Never change branches without permission.
- Use `testify` (`require` for setup/preconditions, `assert` for non-blocking).
- Run `go test ./… -shuffle=on`; don't pass `-count=1` (default).
- Don't pass `-v` to `go test` unless a failure needs it.
- Bun (`bun install`, `bun run …`), never npm.
- Datetimes UTC across DB and API.
- No emojis.
- Vitest is run from `frontend/`, not `packages/ui` (the latter has no vitest config).

---

## Task 1: Lift the repo slice into `runOnce` parameter

**Files:**
- Modify: `internal/github/sync.go`

- [ ] **Step 1.1: Add the parameter without changing behavior**

In `internal/github/sync.go`, locate `func (s *Syncer) runOnce(ctx context.Context, bypassNextSyncAfter bool) {` (line 1051). Change its signature to:

```go
func (s *Syncer) runOnce(
	ctx context.Context,
	bypassNextSyncAfter bool,
	repos []RepoRef,
) {
```

Then delete the inline `s.reposMu.Lock(); … s.reposMu.Unlock()` snapshot block (lines 1065-1068 today) so the function uses the passed `repos` directly. The body's later `total := len(repos)` and `for i, r := range repos` already operate on this slice.

Update the two existing callers within the file so behavior is unchanged:

1. `TriggerRun` (line 516-531):
   ```go
   func (s *Syncer) TriggerRun(ctx context.Context) {
   	s.lifecycleMu.Lock()
   	if s.stopped {
   		s.lifecycleMu.Unlock()
   		return
   	}
   	merged, cancel := s.mergeWithRunCtx(ctx)
   	s.wg.Add(1)
   	s.reposMu.Lock()
   	repos := make([]RepoRef, len(s.repos))
   	copy(repos, s.repos)
   	s.reposMu.Unlock()
   	s.lifecycleMu.Unlock()
   
   	go func() {
   		defer s.wg.Done()
   		defer cancel()
   		s.runOnce(merged, true, repos)
   	}()
   }
   ```

2. The other caller inside `runLoop` at line 1048 (currently `s.runOnce(ctx, false)`). Find it via `grep -n "s.runOnce(ctx" internal/github/sync.go` to confirm context — there should be exactly one other call. Replace with:
   ```go
   s.reposMu.Lock()
   repos := make([]RepoRef, len(s.repos))
   copy(repos, s.repos)
   s.reposMu.Unlock()
   s.runOnce(ctx, false, repos)
   ```

- [ ] **Step 1.2: Build to ensure the refactor compiles**

```bash
cd /home/orenleiman/co/middleman && go build ./internal/github/...
```
Expected: no output (success).

- [ ] **Step 1.3: Run the existing syncer tests**

```bash
cd /home/orenleiman/co/middleman && go test ./internal/github -shuffle=on -short
```
Expected: PASS — the refactor is behavior-preserving, so the existing suite should still be green.

- [ ] **Step 1.4: Commit**

```bash
git add internal/github/sync.go
git commit -m "$(cat <<'EOF'
refactor(sync): lift repos slice into runOnce parameter

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `TriggerRunForRepos` syncer method + unit tests

**Files:**
- Modify: `internal/github/sync.go`
- Modify: `internal/github/sync_test.go`

- [ ] **Step 2.1: Write the failing unit tests**

Open `internal/github/sync_test.go`. Find the existing test helpers and seeding patterns (look for `TestSyncer`, `setupSyncerForTest`, or similar — every test file in this package will have a constructor pattern using `NewSyncer(...)`). Append:

```go
func TestTriggerRunForReposIteratesOnlyPassedSlice(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	// Reuse the test helper this package uses to set up a syncer
	// with two tracked repos and a mock GitHub client. Replace the
	// helper name below with the existing constructor in this file.
	s, mock := setupSyncerForTest(t, []RepoRef{
		{Owner: "acme", Name: "alpha", PlatformHost: "github.com"},
		{Owner: "acme", Name: "beta", PlatformHost: "github.com"},
	})

	require.NoError(s.TriggerRunForRepos(
		context.Background(),
		[]RepoRef{{Owner: "acme", Name: "alpha", PlatformHost: "github.com"}},
	))
	s.wg.Wait()

	calls := mock.PullsCallsFor("acme", "alpha")
	assert.GreaterOrEqual(calls, 1, "alpha should have been synced")

	otherCalls := mock.PullsCallsFor("acme", "beta")
	assert.Zero(otherCalls, "beta should NOT have been synced")
}

func TestTriggerRunForReposEmptySliceIsNoOp(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	s, mock := setupSyncerForTest(t, []RepoRef{
		{Owner: "acme", Name: "alpha", PlatformHost: "github.com"},
	})

	require.NoError(s.TriggerRunForRepos(
		context.Background(), nil,
	))
	s.wg.Wait()

	assert.Zero(mock.PullsCallsFor("acme", "alpha"))
}

func TestTriggerRunForReposRejectsUntrackedRepo(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	s, mock := setupSyncerForTest(t, []RepoRef{
		{Owner: "acme", Name: "alpha", PlatformHost: "github.com"},
	})

	err := s.TriggerRunForRepos(
		context.Background(),
		[]RepoRef{{Owner: "other", Name: "repo", PlatformHost: "github.com"}},
	)
	require.Error(err)
	require.Contains(err.Error(), "not tracked")
	assert.Zero(mock.PullsCallsFor("other", "repo"))
}
```

**Adapt the test helper name and mock methods to match what already exists in `sync_test.go`.** If the existing tests use a different naming convention (e.g. a `MockClient` with `CallCount("acme/alpha")` rather than `PullsCallsFor("acme", "alpha")`), use that. If no helper exists, build a minimal one by mirroring the setup pattern used by an existing test in that file. Do NOT introduce a new mocking framework.

Also add the `context` import if it's not already at the top of `sync_test.go`.

- [ ] **Step 2.2: Run the tests to confirm they fail**

```bash
cd /home/orenleiman/co/middleman && go test ./internal/github -run TestTriggerRunForRepos -shuffle=on
```
Expected: FAIL — `s.TriggerRunForRepos` undefined.

- [ ] **Step 2.3: Implement `TriggerRunForRepos`**

In `internal/github/sync.go`, immediately after `TriggerRun` (line ~531), add:

```go
// TriggerRunForRepos kicks off an ad-hoc sync limited to the given
// repos. Each entry must be in s.repos (case-folded owner+name+host
// match); any unknown entry causes the call to return an error before
// starting the goroutine, so callers see "you can't sync that" rather
// than silently filtering. Empty slice returns nil and is a no-op.
//
// Lifecycle matches TriggerRun: lifecycleMu + mergeWithRunCtx +
// s.wg.Add(1); the spawned goroutine runs runOnce(merged, true, repos)
// and decrements wg/cancel on return. Hard rate-limit pauses, the
// caller's ctx deadline, and Stop semantics carry through unchanged.
func (s *Syncer) TriggerRunForRepos(
	ctx context.Context,
	repos []RepoRef,
) error {
	if len(repos) == 0 {
		return nil
	}
	for _, r := range repos {
		host := r.PlatformHost
		if host == "" {
			host = "github.com"
		}
		if !s.IsTrackedRepoOnHost(r.Owner, r.Name, host) {
			return fmt.Errorf(
				"repo %s/%s on %s is not tracked", r.Owner, r.Name, host,
			)
		}
	}

	// Normalize host on every ref so runOnce sees a clean slice.
	normalized := make([]RepoRef, len(repos))
	for i, r := range repos {
		normalized[i] = r
		if normalized[i].PlatformHost == "" {
			normalized[i].PlatformHost = "github.com"
		}
	}

	s.lifecycleMu.Lock()
	if s.stopped {
		s.lifecycleMu.Unlock()
		return nil
	}
	merged, cancel := s.mergeWithRunCtx(ctx)
	s.wg.Add(1)
	s.lifecycleMu.Unlock()

	go func() {
		defer s.wg.Done()
		defer cancel()
		s.runOnce(merged, true, normalized)
	}()
	return nil
}
```

- [ ] **Step 2.4: Run the tests to confirm they pass**

```bash
cd /home/orenleiman/co/middleman && go test ./internal/github -run TestTriggerRunForRepos -shuffle=on
```
Expected: PASS — three tests.

- [ ] **Step 2.5: Run the full syncer suite**

```bash
cd /home/orenleiman/co/middleman && go test ./internal/github -shuffle=on -short
```
Expected: PASS.

- [ ] **Step 2.6: Commit**

```bash
git add internal/github/sync.go internal/github/sync_test.go
git commit -m "$(cat <<'EOF'
feat(sync): TriggerRunForRepos for scoped ad-hoc sync

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: API endpoint `POST /repos/{owner}/{name}/sync`

**Files:**
- Create: `internal/server/huma_routes_sync.go`
- Modify: `internal/server/huma_routes.go`
- Create: `internal/server/sync_repo_e2e_test.go`

- [ ] **Step 3.1: Write the failing e2e tests**

Create `internal/server/sync_repo_e2e_test.go`:

```go
package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncRepoTriggers202(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	srv, _ := setupTestServer(t)
	client := setupTestClient(t, srv)

	resp, err := client.HTTP.SyncRepoWithResponse(
		context.Background(), "acme", "widget",
	)
	require.NoError(err)
	assert.Equal(http.StatusAccepted, resp.StatusCode())
}

func TestSyncRepoIs403ForUntrackedRepo(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	srv, _ := setupTestServer(t)
	client := setupTestClient(t, srv)

	resp, err := client.HTTP.SyncRepoWithResponse(
		context.Background(), "other", "repo",
	)
	require.NoError(err)
	assert.Equal(http.StatusForbidden, resp.StatusCode())
	assert.Contains(string(resp.Body), "not tracked")
}
```

`setupTestServer` registers `acme/widget` as the tracked repo (see `defaultTestRepos` in `api_test.go`). `other/repo` is intentionally absent and should be rejected as untracked.

- [ ] **Step 3.2: Run the tests to confirm they fail**

```bash
cd /home/orenleiman/co/middleman && go test ./internal/server -run TestSyncRepo -shuffle=on
```
Expected: FAIL — `SyncRepoWithResponse` undefined on the generated client.

- [ ] **Step 3.3: Create the handler file**

Create `internal/server/huma_routes_sync.go`:

```go
package server

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	ghclient "github.com/wesm/middleman/internal/github"
)

// --- inputs ------------------------------------------------------------------

type syncRepoInput struct {
	Owner string `path:"owner"`
	Name  string `path:"name"`
}

// --- handlers ----------------------------------------------------------------

// triggerSync kicks off a full sync over every configured repo.
// Identical to the previous implementation in huma_routes.go; moved
// here to colocate sync handlers.
func (s *Server) triggerSync(
	ctx context.Context, _ *struct{},
) (*acceptedOutput, error) {
	s.syncer.TriggerRun(context.WithoutCancel(ctx))
	return &acceptedOutput{Status: http.StatusAccepted}, nil
}

// syncRepo kicks off an ad-hoc sync limited to one repo. The repo
// must already be in the configured list; untracked repos return 403
// to match the convention used by syncPR. The host is resolved via
// Syncer.hostFor (which defaults unknown owner/name pairs to
// github.com) before the tracked-set check.
func (s *Server) syncRepo(
	ctx context.Context, input *syncRepoInput,
) (*acceptedOutput, error) {
	host := s.syncer.HostFor(input.Owner, input.Name)
	if !s.syncer.IsTrackedRepoOnHost(input.Owner, input.Name, host) {
		return nil, huma.Error403Forbidden(
			"repo " + input.Owner + "/" + input.Name +
				" on " + host + " is not tracked",
		)
	}
	if err := s.syncer.TriggerRunForRepos(
		context.WithoutCancel(ctx),
		[]ghclient.RepoRef{{
			Owner:        input.Owner,
			Name:         input.Name,
			PlatformHost: host,
		}},
	); err != nil {
		return nil, huma.Error500InternalServerError(
			"trigger repo sync: " + err.Error(),
		)
	}
	return &acceptedOutput{Status: http.StatusAccepted}, nil
}
```

`Syncer.hostFor` is currently unexported (`internal/github/sync.go:595`). Promote it to `HostFor`:

In `internal/github/sync.go`, find `func (s *Syncer) hostFor(owner, name string) string {` (around line 595). Rename to `HostFor`:

```go
// HostFor returns the platform host for a repo identified by
// owner/name. Returns "github.com" if not found. Thread-safe.
func (s *Syncer) HostFor(owner, name string) string {
```

Then update any in-package callers. Find them:

```bash
grep -rn "\.hostFor(" /home/orenleiman/co/middleman/internal/github/
```

Replace each `s.hostFor(...)` (or `syncer.hostFor(...)`) with the capitalized `HostFor` form.

- [ ] **Step 3.4: Register the new route and remove the old handler**

In `internal/server/huma_routes.go`:

1. **Delete** the existing `triggerSync` function (lines ~1930-1933) — it's moved to the new file.
2. Find the existing route registration for `/sync` (around line 412-417):

   ```go
   huma.Register(api, huma.Operation{
       OperationID:   "trigger-sync",
       Method:        http.MethodPost,
       Path:          "/sync",
       DefaultStatus: http.StatusAccepted,
   }, s.triggerSync)
   ```

   Immediately below it, add the new registration:

   ```go
   huma.Register(api, huma.Operation{
       OperationID:   "sync-repo",
       Method:        http.MethodPost,
       Path:          "/repos/{owner}/{name}/sync",
       DefaultStatus: http.StatusAccepted,
   }, s.syncRepo)
   ```

3. Confirm `triggerSync` is no longer referenced anywhere else in `huma_routes.go`:

   ```bash
   grep -n "triggerSync\b" /home/orenleiman/co/middleman/internal/server/huma_routes.go
   ```

   Only the registration block should reference it.

- [ ] **Step 3.5: Regenerate OpenAPI + clients**

```bash
cd /home/orenleiman/co/middleman && make api-generate
```
Expected: regenerates the five artifacts in the file map, adds `sync-repo` operation + `SyncRepoWithResponse` to `client.gen.go`.

If `make api-generate` fails in the sandbox, retry the call with `dangerouslyDisableSandbox: true`.

- [ ] **Step 3.6: Run the new tests**

```bash
cd /home/orenleiman/co/middleman && go test ./internal/server -run TestSyncRepo -shuffle=on
```
Expected: PASS — two tests.

- [ ] **Step 3.7: Run the broader server suite to catch regressions**

```bash
cd /home/orenleiman/co/middleman && go test ./internal/server -shuffle=on -short
```
Expected: PASS. (Note: `TestAPIActivityReturnsUTCCreatedAt` is a known pre-existing failure unrelated to this work — ignore it. Anything else failing is a regression.)

- [ ] **Step 3.8: Commit**

```bash
git add internal/github/sync.go \
        internal/server/huma_routes_sync.go \
        internal/server/huma_routes.go \
        internal/server/sync_repo_e2e_test.go \
        frontend/openapi/openapi.json \
        internal/apiclient/spec/openapi.json \
        internal/apiclient/generated/client.gen.go \
        packages/ui/src/api/generated/schema.ts \
        packages/ui/src/api/generated/client.ts
git commit -m "$(cat <<'EOF'
feat(server): POST /repos/{owner}/{name}/sync for scoped sync

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Shared `apiErrorMessage` helper

**Files:**
- Create: `packages/ui/src/api/errors.ts`
- Modify: `packages/ui/src/stores/detail.svelte.ts`

`apiErrorMessage` is currently private to `detail.svelte.ts:50`. Extract it so `sync.svelte.ts` and `diff.svelte.ts` can use the same pattern.

- [ ] **Step 4.1: Create the shared helper**

Create `packages/ui/src/api/errors.ts`:

```ts
// apiErrorMessage extracts a human-readable message from an
// openapi-fetch error result. Returns the explicit `detail`, falling
// back to `title`, then to the supplied fallback. Use the fallback to
// describe what the failed action was trying to do (e.g. "sync
// failed").
export function apiErrorMessage(
  error: { detail?: string; title?: string } | undefined,
  fallback: string,
): string {
  if (!error) return fallback;
  return error.detail ?? error.title ?? fallback;
}
```

- [ ] **Step 4.2: Update `detail.svelte.ts` to import the shared helper**

In `packages/ui/src/stores/detail.svelte.ts`:

1. Add to the top imports:
   ```ts
   import { apiErrorMessage } from "../api/errors.js";
   ```

2. Delete the local `function apiErrorMessage(...)` definition (currently around line 46-56).

- [ ] **Step 4.3: Run detail-store tests + typecheck**

```bash
cd /home/orenleiman/co/middleman/frontend && bun run vitest run ../packages/ui/src/stores/detail.test.ts
cd /home/orenleiman/co/middleman/packages/ui && bun run typecheck
```
Expected: PASS, 0 type errors.

- [ ] **Step 4.4: Commit**

```bash
git add packages/ui/src/api/errors.ts packages/ui/src/stores/detail.svelte.ts
git commit -m "$(cat <<'EOF'
refactor(ui): extract apiErrorMessage helper into shared module

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Sync store — `triggerSyncForRepo`

**Files:**
- Modify: `packages/ui/src/stores/sync.svelte.ts`
- Create: `packages/ui/src/stores/sync.test.ts`

- [ ] **Step 5.1: Write the failing store test**

Create `packages/ui/src/stores/sync.test.ts`:

```ts
import { afterEach, describe, expect, it, vi } from "vitest";
import { createSyncStore } from "./sync.svelte.js";
import type { MiddlemanClient } from "../types.js";

function makeStubClient(opts: {
  postResponse?: { data?: unknown; error?: { detail?: string } };
} = {}): MiddlemanClient {
  return {
    GET: vi.fn(async () => ({ data: undefined, error: undefined })),
    POST: vi.fn(async () =>
      opts.postResponse ?? { data: undefined, error: undefined },
    ),
    DELETE: vi.fn(async () => ({ data: undefined, error: undefined })),
  } as unknown as MiddlemanClient;
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("triggerSyncForRepo", () => {
  it("POSTs to /repos/{owner}/{name}/sync with the right path params", async () => {
    const client = makeStubClient();
    const store = createSyncStore({ client });

    await store.triggerSyncForRepo("acme", "widget");

    expect(client.POST).toHaveBeenCalledWith(
      "/repos/{owner}/{name}/sync",
      expect.objectContaining({
        params: { path: { owner: "acme", name: "widget" } },
      }),
    );
  });

  it("sets last_error on API failure", async () => {
    const client = makeStubClient({
      postResponse: { data: undefined, error: { detail: "boom" } },
    });
    const store = createSyncStore({ client });

    await expect(
      store.triggerSyncForRepo("acme", "widget"),
    ).rejects.toThrow("boom");

    expect(store.getSyncState()?.last_error).toBe("boom");
  });
});
```

- [ ] **Step 5.2: Run the test to confirm it fails**

```bash
cd /home/orenleiman/co/middleman/frontend && bun run vitest run ../packages/ui/src/stores/sync.test.ts
```
Expected: FAIL — `triggerSyncForRepo` undefined.

- [ ] **Step 5.3: Implement `triggerSyncForRepo`**

Edit `packages/ui/src/stores/sync.svelte.ts`.

1. Add to the top imports:
   ```ts
   import { apiErrorMessage } from "../api/errors.js";
   ```

2. Immediately after the existing `async function triggerSync(): Promise<void>` (around line 108-142), add:

   ```ts
   async function triggerSyncForRepo(
     owner: string,
     name: string,
   ): Promise<void> {
     const previous = status;

     status = {
       running: true,
       last_run_at: previous?.last_run_at ?? "",
       last_error: "",
     };
     wasRunning = true;
     adjustPollingSpeed(true);

     try {
       const { error } = await apiClient.POST(
         "/repos/{owner}/{name}/sync",
         { params: { path: { owner, name } } },
       );
       if (error) {
         throw new Error(
           apiErrorMessage(error, "failed to trigger sync"),
         );
       }
       await refreshSyncStatus();
     } catch (err) {
       status = {
         running: false,
         last_run_at: previous?.last_run_at ?? "",
         last_error:
           err instanceof Error
             ? err.message
             : "failed to trigger sync",
       };
       wasRunning = false;
       adjustPollingSpeed(false);
       throw err;
     }
   }
   ```

3. Add `triggerSyncForRepo` to the returned object (around line 175-185):

   ```ts
   return {
     getSyncState,
     getRateLimits,
     onNextSyncComplete,
     subscribeSyncComplete,
     refreshSyncStatus,
     setSyncStatus,
     triggerSync,
     triggerSyncForRepo,
     startPolling,
     stopPolling,
   };
   ```

- [ ] **Step 5.4: Run the tests to confirm they pass**

```bash
cd /home/orenleiman/co/middleman/frontend && bun run vitest run ../packages/ui/src/stores/sync.test.ts
cd /home/orenleiman/co/middleman/packages/ui && bun run typecheck
```
Expected: PASS (two tests), 0 type errors.

- [ ] **Step 5.5: Commit**

```bash
git add packages/ui/src/stores/sync.svelte.ts packages/ui/src/stores/sync.test.ts
git commit -m "$(cat <<'EOF'
feat(ui): triggerSyncForRepo for scoped sync action

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: AppHeader scope-aware Sync button

**Files:**
- Modify: `frontend/src/lib/components/layout/AppHeader.svelte`
- Modify: `frontend/src/lib/components/layout/AppHeader.test.ts`

`getGlobalRepo()` returns `string | undefined` shaped as `"owner/name"`. The handler must split on `/`.

- [ ] **Step 6.1: Write the failing test (extend the existing file)**

Open `frontend/src/lib/components/layout/AppHeader.test.ts` and read it first to understand how `getStores` / `getGlobalRepo` are mocked. Then append (placement inside an existing `describe(...)` block is fine):

```ts
import { setGlobalRepo } from "../../stores/filter.svelte.js";
// (if not already imported above; check the existing top-of-file imports)

describe("AppHeader scoped sync", () => {
  it("calls triggerSyncForRepo with the selected owner/name", async () => {
    setGlobalRepo("acme/widget");

    // Render AppHeader with a stores context whose sync stub records calls.
    // Mirror the rendering pattern that the existing tests use; the stub
    // should expose:
    //   triggerSync: vi.fn(async () => {}),
    //   triggerSyncForRepo: vi.fn(async () => {}),
    //   getSyncState: () => null,
    const sync = {
      triggerSync: vi.fn(async () => {}),
      triggerSyncForRepo: vi.fn(async () => {}),
      getSyncState: () => null,
    };
    renderAppHeader({ sync });

    await fireEvent.click(screen.getByRole("button", { name: /Sync/i }));
    expect(sync.triggerSyncForRepo).toHaveBeenCalledWith("acme", "widget");
    expect(sync.triggerSync).not.toHaveBeenCalled();
  });

  it("falls back to full sync when no repo is selected", async () => {
    setGlobalRepo(undefined);
    const sync = {
      triggerSync: vi.fn(async () => {}),
      triggerSyncForRepo: vi.fn(async () => {}),
      getSyncState: () => null,
    };
    renderAppHeader({ sync });

    await fireEvent.click(screen.getByRole("button", { name: /Sync/i }));
    expect(sync.triggerSync).toHaveBeenCalled();
    expect(sync.triggerSyncForRepo).not.toHaveBeenCalled();
  });
});
```

Adapt the `renderAppHeader({ sync })` call to match the existing test file's helper. If no helper exists, look at how the file currently mounts `AppHeader.svelte` (probably via `render(AppHeader, { context: new Map([[STORES_KEY, { sync }]]) })`) and follow that pattern.

Also import `fireEvent`, `screen`, and `vi` at the top of the file if not already present:

```ts
import { fireEvent, screen } from "@testing-library/svelte";
import { describe, expect, it, vi } from "vitest";
```

- [ ] **Step 6.2: Run the tests to confirm they fail**

```bash
cd /home/orenleiman/co/middleman/frontend && bun run vitest run src/lib/components/layout/AppHeader.test.ts -t "scoped sync"
```
Expected: FAIL — `triggerSyncForRepo` not called (or button label changed so the role query doesn't match).

- [ ] **Step 6.3: Update `AppHeader.svelte`**

Edit `frontend/src/lib/components/layout/AppHeader.svelte`.

Replace the existing `handleSync` block (around lines 26-29):

```svelte
  async function handleSync(): Promise<void> {
    if (sync.getSyncState()?.running) return;
    const selected = getGlobalRepo();
    if (selected) {
      const slash = selected.indexOf("/");
      if (slash > 0 && slash < selected.length - 1) {
        const owner = selected.slice(0, slash);
        const name = selected.slice(slash + 1);
        await sync.triggerSyncForRepo(owner, name);
        return;
      }
    }
    await sync.triggerSync();
  }

  const selectedRepo = $derived(getGlobalRepo());
```

Then replace the Sync button (around lines 122-126):

```svelte
    {#if !getUIConfig().hideSync}
      <button class="action-btn" onclick={handleSync} disabled={syncing}>
        {#if selectedRepo}
          {syncing ? "Syncing" : "Sync"}
          <span class="sync-scope">{selectedRepo}</span>
          {syncing ? "…" : ""}
        {:else}
          {syncing ? "Syncing..." : "Sync"}
        {/if}
      </button>
    {/if}
```

Add the truncation style near the existing `.action-btn` rules in the `<style>` block:

```css
  .sync-scope {
    display: inline-block;
    max-width: 160px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    vertical-align: bottom;
    margin-left: 4px;
    font-weight: 600;
  }
```

- [ ] **Step 6.4: Run the tests to confirm they pass**

```bash
cd /home/orenleiman/co/middleman/frontend && bun run vitest run src/lib/components/layout/AppHeader.test.ts
```
Expected: PASS — both new tests and any existing ones.

- [ ] **Step 6.5: Typecheck**

```bash
cd /home/orenleiman/co/middleman/frontend && bun run typecheck
```
Expected: 0 errors.

- [ ] **Step 6.6: Commit**

```bash
git add frontend/src/lib/components/layout/AppHeader.svelte \
        frontend/src/lib/components/layout/AppHeader.test.ts
git commit -m "$(cat <<'EOF'
feat(ui): scope the header Sync button to the selected repo

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Diff store — `refresh()` switches to typed client

**Files:**
- Modify: `packages/ui/src/stores/diff.svelte.ts`
- Modify: `packages/ui/src/Provider.svelte`
- Create: `packages/ui/src/stores/diff.refresh.test.ts`

The 415 root cause: `refresh()` uses `fetch(url, { method: "POST" })` with no `Content-Type`, which the server's CSRF middleware rejects.

- [ ] **Step 7.1: Write the failing test**

Create `packages/ui/src/stores/diff.refresh.test.ts`:

```ts
import { afterEach, describe, expect, it, vi } from "vitest";
import { createDiffStore } from "./diff.svelte.js";
import type { MiddlemanClient } from "../types.js";

function makeStubClient(opts: {
  postResponse?: { data?: unknown; error?: { detail?: string } };
} = {}): MiddlemanClient {
  return {
    GET: vi.fn(async () => ({ data: undefined, error: undefined })),
    POST: vi.fn(async () =>
      opts.postResponse ?? { data: undefined, error: undefined },
    ),
    DELETE: vi.fn(async () => ({ data: undefined, error: undefined })),
  } as unknown as MiddlemanClient;
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("diff store refresh", () => {
  it("POSTs to the typed PR sync endpoint with the right path params", async () => {
    const client = makeStubClient();
    const store = createDiffStore({ client });
    store.setActivePR("acme", "widget", 42);

    await store.refresh();

    expect(client.POST).toHaveBeenCalledWith(
      "/repos/{owner}/{name}/pulls/{number}/sync",
      expect.objectContaining({
        params: { path: { owner: "acme", name: "widget", number: 42 } },
      }),
    );
  });

  it("sets refreshError from the server detail on failure", async () => {
    const client = makeStubClient({
      postResponse: { data: undefined, error: { detail: "boom" } },
    });
    const store = createDiffStore({ client });
    store.setActivePR("acme", "widget", 42);

    await store.refresh();

    expect(store.getRefreshError()).toBe("boom");
  });
});
```

If `setActivePR` is not the actual method name on the diff store, find the existing setter (look for "currentOwner", "setRepo", or similar) and use that. The diff store has internal state for the currently-loaded PR; the test needs to seed it so `refresh()` knows what to refresh.

If no public setter exists, you'll need to add one (small change) — keep it limited to what `refresh()` needs:

```ts
function setActivePR(owner: string, name: string, number: number): void {
  currentOwner = owner;
  currentName = name;
  currentNumber = number;
}
```

…and expose it in the returned object alongside `refresh`.

- [ ] **Step 7.2: Run the test to confirm it fails**

```bash
cd /home/orenleiman/co/middleman/frontend && bun run vitest run ../packages/ui/src/stores/diff.refresh.test.ts
```
Expected: FAIL — `createDiffStore({ client })` rejects an unknown option (the store doesn't take `client` yet), or refresh() still uses raw fetch.

- [ ] **Step 7.3: Thread the typed client into the diff store**

Edit `packages/ui/src/stores/diff.svelte.ts`.

1. Update the import block at the top:
   ```ts
   import type { MiddlemanClient } from "../types.js";
   import { apiErrorMessage } from "../api/errors.js";
   ```

2. Update `DiffStoreOptions`:
   ```ts
   export interface DiffStoreOptions {
     client: MiddlemanClient;
     getBasePath?: () => string;
   }
   ```

3. In `createDiffStore`, change the constructor body to require the client:
   ```ts
   export function createDiffStore(opts: DiffStoreOptions) {
     const apiClient = opts.client;
     const getBasePath = opts.getBasePath ?? (() => "/");
     // ... rest of function ...
   ```

4. Rewrite `refresh()` (currently lines 379-409). Replace its body with:

   ```ts
   async function refresh(): Promise<void> {
     if (!currentOwner || refreshing) return;
     refreshing = true;
     refreshError = null;
     try {
       const { error } = await apiClient.POST(
         "/repos/{owner}/{name}/pulls/{number}/sync",
         {
           params: {
             path: {
               owner: currentOwner,
               name: currentName,
               number: currentNumber,
             },
           },
         },
       );
       if (error) {
         refreshError = apiErrorMessage(error, "sync failed");
         return;
       }
     } catch (err) {
       refreshError = err instanceof Error ? err.message : String(err);
       return;
     } finally {
       refreshing = false;
     }
     // Clear the commit cache so loadCommits re-fetches against the
     // new head SHAs that the sync just wrote to the DB.
     commits = null;
     commitsError = null;
     patchsets = null;
     patchsetsError = null;
     void loadCommits();
     void reloadDiffOnly();
   }
   ```

5. If you added a `setActivePR` helper for the test, add it to the returned object near `refresh`.

- [ ] **Step 7.4: Thread the client into the diff store at construction**

Edit `packages/ui/src/Provider.svelte`.

Find the diff store construction (around line 205-210):

```svelte
const diffOpts: DiffStoreOptions = {};
if (cfg.basePath != null) {
  const bp = cfg.basePath;
  diffOpts.getBasePath = () => bp;
}
const diffStore = createDiffStore(diffOpts);
```

Replace with:

```svelte
const diffOpts: DiffStoreOptions = { client: cl };
if (cfg.basePath != null) {
  const bp = cfg.basePath;
  diffOpts.getBasePath = () => bp;
}
const diffStore = createDiffStore(diffOpts);
```

`cl` is the `MiddlemanClient` instance already constructed earlier in this script block (used by other stores like `worktreesStore`).

- [ ] **Step 7.5: Run the tests to confirm they pass**

```bash
cd /home/orenleiman/co/middleman/frontend && bun run vitest run ../packages/ui/src/stores/diff.refresh.test.ts
cd /home/orenleiman/co/middleman/packages/ui && bun run typecheck
```
Expected: PASS, 0 type errors. The full diff component suite:

```bash
cd /home/orenleiman/co/middleman/frontend && bun run vitest run ../packages/ui/src/components/diff
```

Expected: PASS. If any of the existing diff tests construct `createDiffStore(...)` without a client, they need a stub client too — update those call sites to match the new required option.

- [ ] **Step 7.6: Commit**

```bash
git add packages/ui/src/stores/diff.svelte.ts \
        packages/ui/src/Provider.svelte \
        packages/ui/src/stores/diff.refresh.test.ts
git commit -m "$(cat <<'EOF'
fix(ui): diff store refresh uses typed client, sending Content-Type

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: DiffToolbar surfaces the actual error text

**Files:**
- Modify: `packages/ui/src/components/diff/DiffToolbar.svelte`

- [ ] **Step 8.1: Update the error pill markup and styles**

Open `packages/ui/src/components/diff/DiffToolbar.svelte`. Find the error pill (around lines 96-98):

```svelte
{#if diff.getRefreshError()}
  <span class="refresh-error" title={diff.getRefreshError()}>sync failed</span>
{/if}
```

Replace with:

```svelte
{#if diff.getRefreshError()}
  <span class="refresh-error" title={diff.getRefreshError()}>
    {diff.getRefreshError()}
  </span>
{/if}
```

Find the existing `.refresh-error` rule in the `<style>` block (search for `.refresh-error`). Add (or extend) it to include truncation:

```css
  .refresh-error {
    display: inline-block;
    max-width: 220px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    /* keep any color/padding rules that were already on .refresh-error */
  }
```

If `.refresh-error` already has a `color: var(--accent-red)` / padding / font-size declaration, preserve those. Add only the four overflow-related properties.

- [ ] **Step 8.2: Run the diff component suite to catch any breakage**

```bash
cd /home/orenleiman/co/middleman/frontend && bun run vitest run ../packages/ui/src/components/diff
cd /home/orenleiman/co/middleman/packages/ui && bun run typecheck
```
Expected: PASS.

- [ ] **Step 8.3: Commit**

```bash
git add packages/ui/src/components/diff/DiffToolbar.svelte
git commit -m "$(cat <<'EOF'
feat(ui): surface actual refresh error in DiffToolbar pill

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Full-stack smoke verification

**Files:** none modified

- [ ] **Step 9.1: Run the full Go suite**

```bash
cd /home/orenleiman/co/middleman && make test
```
Expected: PASS. Known pre-existing failure: `TestAPIActivityReturnsUTCCreatedAt`. Anything else failing is a regression — go fix it before continuing.

- [ ] **Step 9.2: Run lint**

```bash
cd /home/orenleiman/co/middleman && make lint
```
Expected: PASS. (Environment caveat: `make lint` may require `mise`-pinned `golangci-lint`. If it fails for tooling reasons rather than a real lint issue, note that and proceed.)

- [ ] **Step 9.3: Frontend tests + typechecks**

```bash
cd /home/orenleiman/co/middleman/frontend && bun run test
cd /home/orenleiman/co/middleman/packages/ui && bun run typecheck
cd /home/orenleiman/co/middleman/frontend && bun run typecheck
```
Expected: PASS for typechecks. Known pre-existing AppHeader theme/localStorage failures may still be present — anything new is a regression.

- [ ] **Step 9.4: Manual UI verification**

Per CLAUDE.md "For UI or frontend changes, start the dev server and use the feature in a browser before reporting the task as complete."

In two terminals:

```bash
make dev
make frontend-dev
```

Steps:

1. **Selected-repo sync.** With a tracked repo selected in the header dropdown, click Sync. Confirm:
   - Network tab shows `POST /api/v1/repos/<owner>/<name>/sync` returning 202.
   - The button label reads `Sync <owner>/<name>` (or `Syncing <owner>/<name>…` during the run).
   - Only that repo's PRs refresh; the slow repo is left alone.
2. **Full sync.** Clear the dropdown selection (or set it to "all repos"). Click Sync. Confirm:
   - Network tab shows `POST /api/v1/sync` returning 202.
   - Button label is plain `Sync` / `Syncing...`.
3. **Untracked repo guard.** Hand-craft a POST in the browser console to `POST /api/v1/repos/other/repo/sync` with `Content-Type: application/json`. Confirm 403 with `"is not tracked"` in the body.
4. **Refresh button works.** Open any PR's diff view. Click Refresh in the toolbar. Confirm:
   - Network tab shows `POST /api/v1/repos/<owner>/<name>/pulls/<n>/sync` returning 200 (previously 415).
   - Diff + commits reload.
   - Pre-fix: clicking Refresh produced "sync failed" pill. Now: pill is absent on success.
5. **Refresh error visibility.** With `make dev` still running, kill it; the next Refresh click should produce a visible error message (whatever the network failure says) in the pill, not the literal "sync failed".

If anything misbehaves, do not declare done. Treat the deviation as a Task 9 failure and add a follow-up task.

- [ ] **Step 9.5: Optional final tidy commit**

If any minor adjustments were needed during 9.4 (typos, CSS tweaks), commit them with a focused message. Otherwise skip.

---

## Spec Coverage Audit

Walking the spec section by section:

- **Syncer refactor (`runOnce` takes `repos`, new `TriggerRunForRepos`):** Tasks 1 & 2.
- **API endpoint `POST /repos/{owner}/{name}/sync` with 403 on untracked:** Task 3 (steps 3.3-3.4 + tests at 3.1).
- **`Syncer.HostFor` promotion:** Task 3 Step 3.3.
- **Frontend `triggerSyncForRepo` store action with `apiErrorMessage`:** Task 5.
- **Shared `apiErrorMessage` extraction:** Task 4.
- **AppHeader conditional dispatch + scoped button label + truncation:** Task 6.
- **Background / watched-PR sync unchanged:** Confirmed by leaving `watchLoop`-related code untouched across Tasks 1-2.
- **Sync-status indicator unchanged:** No code change; the existing `getSyncState()` reflects whichever run just fired.
- **Refresh fix: switch raw `fetch` to typed `apiClient.POST`:** Task 7.
- **Refresh visible-error UX (replace "sync failed" literal + truncation):** Task 8.
- **Future extensions (persistent skip, multi-select dropdown, per-PR header button):** explicitly out of scope per spec; no tasks.
- **Tests Go side (syncer unit + e2e):** Tasks 2.1 and 3.1.
- **Tests frontend (sync store, diff refresh, AppHeader):** Tasks 5.1, 7.1, 6.1.
- **Manual smoke:** Task 9.4.

No gaps.

## Placeholder Audit

Scanned the plan for the "no placeholders" red flags. No "TBD" / "TODO" / "implement later" / "add validation" lines. Two adaptive instructions reference existing code:

- Task 2 Step 2.1 says "adapt the test helper name and mock methods to match what already exists in `sync_test.go`." This is necessary because the existing test file's helper conventions weren't fully read during planning. Each adaptation is concrete (helper name, mock method shape) with a fallback plan ("if no helper exists, build a minimal one by mirroring the setup pattern"). Not a placeholder.
- Task 7 Step 7.1 says "if `setActivePR` is not the actual method name…use that." Same shape — necessary because `createDiffStore`'s internal state setters weren't fully audited. Fallback is to add a small setter, with the exact code shown.

Both are conditional rather than vague; an engineer following the plan can act without further clarification.

## Type / Name Consistency

- `TriggerRunForRepos(ctx, []RepoRef) error` — same signature in Task 1 (callsite) and Task 2 (definition).
- `Syncer.HostFor` (renamed from `hostFor`) — promoted in Task 3 Step 3.3, used in Task 3 Step 3.3.
- `acceptedOutput` — reused from existing `huma_routes.go`; Task 3 references it without redefining.
- `syncRepoInput` — defined in Task 3 Step 3.3 only.
- `triggerSyncForRepo(owner: string, name: string): Promise<void>` — same shape in Task 5 (definition) and Task 6 (callsite).
- `apiErrorMessage(error, fallback)` — defined in Task 4 Step 4.1; imported by Task 4 Step 4.2 (detail.svelte.ts), Task 5 Step 5.3 (sync.svelte.ts), Task 7 Step 7.3 (diff.svelte.ts).
- `DiffStoreOptions.client: MiddlemanClient` — required in Task 7 Step 7.3 (definition) and supplied in Task 7 Step 7.4 (Provider.svelte).
- Generated client method `SyncRepoWithResponse` — referenced in Task 3 Step 3.1 test and produced by Task 3 Step 3.5 (`make api-generate`).

No drift detected.
