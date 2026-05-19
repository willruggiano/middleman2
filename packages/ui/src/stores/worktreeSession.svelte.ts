import type { MiddlemanClient } from "../types.js";
import type { components } from "../api/generated/schema.js";

// Frontend mirror of the server-side session shapes. Both come from
// the same generated OpenAPI types so changes on either side surface
// at typecheck.
export type SessionTurn = components["schemas"]["SessionTurnResponse"];
export type Session = components["schemas"]["SessionResponse"];

export interface WorktreeSessionStoreOptions {
  client: MiddlemanClient;
}

interface SessionEntry {
  session: Session | null;
  turns: SessionTurn[];
  loading: boolean;
  error: string | null;
  // pollHandle isn't reactive; keep it outside $state to avoid
  // accidentally re-rendering when the interval id mutates.
}

// The session store keys all state by (owner, name, number). For
// local worktrees that means owner="local", name=basename of the
// configured local_path, number=worktree id. Same scheme the rest
// of the review machinery uses.
//
// Polling: while any claude_response turn is queued or running, we
// re-fetch the session every 1.5s so the UI animates the spinner +
// flips to the final content without the user having to refresh.
export function createWorktreeSessionStore(opts: WorktreeSessionStoreOptions) {
  const client = opts.client;

  // Single-active-session model — we only ever track one
  // (owner,name,number) at a time. Selection changes wipe the slot.
  let key = $state<string | null>(null);
  let entry = $state<SessionEntry>({
    session: null,
    turns: [],
    loading: false,
    error: null,
  });
  let pollHandle: ReturnType<typeof setInterval> | null = null;

  function getSession(): Session | null {
    return entry.session;
  }
  function getTurns(): SessionTurn[] {
    return entry.turns;
  }
  function isLoading(): boolean {
    return entry.loading;
  }
  function getError(): string | null {
    return entry.error;
  }
  function getKey(): string | null {
    return key;
  }
  function hasRunningTurn(): boolean {
    return entry.turns.some(
      (t) => t.status === "running" || t.status === "queued",
    );
  }

  function buildKey(owner: string, name: string, number: number): string {
    return `${owner}/${name}/${number}`;
  }

  function stopPolling(): void {
    if (pollHandle !== null) {
      clearInterval(pollHandle);
      pollHandle = null;
    }
  }

  function startPollingIfNeeded(
    owner: string, name: string, number: number,
  ): void {
    stopPolling();
    if (!hasRunningTurn()) return;
    pollHandle = setInterval(() => {
      void loadSession(owner, name, number, { silent: true });
    }, 1500);
  }

  async function loadSession(
    owner: string,
    name: string,
    number: number,
    opts: { silent?: boolean } = {},
  ): Promise<void> {
    const newKey = buildKey(owner, name, number);
    if (newKey !== key) {
      // Selection changed — wipe previous state immediately so the
      // user doesn't see another session's turns flash on screen.
      stopPolling();
      key = newKey;
      entry = { session: null, turns: [], loading: false, error: null };
    }
    if (!opts.silent) {
      entry = { ...entry, loading: true, error: null };
    }
    try {
      const { data, error } = await client.GET(
        "/repos/{owner}/{name}/pulls/{number}/session",
        { params: { path: { owner, name, number } } },
      );
      if (key !== newKey) return; // selection moved on
      if (error) {
        throw new Error(
          (error as { detail?: string }).detail ?? "failed to load session",
        );
      }
      // OpenAPI flattens nullable values; an absent session shows
      // up as a zero-id record. Detect and normalize back to null.
      const sess = data?.session;
      const sessionValue =
        sess && sess.id !== 0 ? sess : null;
      entry = {
        session: sessionValue,
        turns: data?.turns ?? [],
        loading: false,
        error: null,
      };
      startPollingIfNeeded(owner, name, number);
    } catch (err) {
      if (key !== newKey) return;
      entry = {
        ...entry,
        loading: false,
        error: err instanceof Error ? err.message : String(err),
      };
    }
  }

  async function submitTurn(
    owner: string,
    name: string,
    number: number,
    body: { type: "review_feedback" | "user_message"; content: string; metadataJSON?: string },
  ): Promise<void> {
    const newKey = buildKey(owner, name, number);
    try {
      const reqBody: {
        type: "review_feedback" | "user_message";
        content: string;
        metadata_json?: string;
      } = { type: body.type, content: body.content };
      if (body.metadataJSON !== undefined) {
        reqBody.metadata_json = body.metadataJSON;
      }
      const { data, error } = await client.POST(
        "/repos/{owner}/{name}/pulls/{number}/session/turns",
        {
          params: { path: { owner, name, number } },
          body: reqBody,
        },
      );
      if (error) {
        throw new Error(
          (error as { detail?: string }).detail ?? "failed to submit turn",
        );
      }
      if (key !== newKey) {
        key = newKey;
      }
      // Append the two new turn rows the server inserted, plus
      // refresh the session row. Subsequent polls will flip the
      // response turn to done/failed.
      const session =
        data?.session && data.session.id !== 0 ? data.session : null;
      const userTurn = data?.user_turn;
      const responseTurn = data?.response_turn;
      const next: SessionTurn[] = [...entry.turns];
      if (userTurn) next.push(userTurn);
      if (responseTurn) next.push(responseTurn);
      entry = { ...entry, session, turns: next, error: null };
      startPollingIfNeeded(owner, name, number);
    } catch (err) {
      entry = {
        ...entry,
        error: err instanceof Error ? err.message : String(err),
      };
      throw err;
    }
  }

  async function killSession(
    owner: string,
    name: string,
    number: number,
  ): Promise<void> {
    try {
      const { error } = await client.POST(
        "/repos/{owner}/{name}/pulls/{number}/session/kill",
        {
          params: { path: { owner, name, number } },
        },
      );
      if (error) {
        throw new Error(
          (error as { detail?: string }).detail ?? "failed to kill session",
        );
      }
      // The session is gone; clear local state so the empty
      // placeholder renders. A subsequent submitTurn will create
      // a fresh session.
      stopPolling();
      entry = { session: null, turns: [], loading: false, error: null };
    } catch (err) {
      entry = {
        ...entry,
        error: err instanceof Error ? err.message : String(err),
      };
    }
  }

  async function cancelTurn(
    owner: string,
    name: string,
    number: number,
    turnID: number,
  ): Promise<void> {
    try {
      const { error } = await client.POST(
        "/repos/{owner}/{name}/pulls/{number}/session/turns/{turn_id}/cancel",
        {
          params: { path: { owner, name, number, turn_id: turnID } },
        },
      );
      if (error) {
        throw new Error(
          (error as { detail?: string }).detail ?? "failed to cancel turn",
        );
      }
      void loadSession(owner, name, number, { silent: true });
    } catch (err) {
      entry = {
        ...entry,
        error: err instanceof Error ? err.message : String(err),
      };
    }
  }

  function clear(): void {
    stopPolling();
    key = null;
    entry = { session: null, turns: [], loading: false, error: null };
  }

  return {
    getSession,
    getTurns,
    isLoading,
    getError,
    getKey,
    hasRunningTurn,
    loadSession,
    submitTurn,
    cancelTurn,
    killSession,
    clear,
  };
}

export type WorktreeSessionStore = ReturnType<typeof createWorktreeSessionStore>;
