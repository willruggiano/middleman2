import type { MiddlemanClient } from "../types.js";

export interface AISessionThread {
  id: number;
  mrId: number;
  repoOwner: string;
  repoName: string;
  mrNumber: number;
  mrTitle: string;
  path: string;
  anchorSide: "LEFT" | "RIGHT";
  anchorLine: number;
  createdAt: string;
  latestQuestionStatus: string;
  openQuestionCount: number;
  hasWorktree: boolean;
}

export interface AISessionBrief {
  id: number;
  mrId: number;
  repoOwner: string;
  repoName: string;
  mrNumber: number;
  mrTitle: string;
  status: string;
  depth: string;
  createdAt: string;
  startedAt: string;
}

export interface AISessionsStoreOptions {
  client: MiddlemanClient;
  // onThreadDeleted fires after a successful server-side close so
  // the per-PR aiStore can mirror the deletion in its local state.
  // Without it, the diff view's thread cards keep rendering until
  // aiStore's next refresh — which is gated on in-flight questions
  // and so never happens for an idle thread.
  onThreadDeleted?: (threadID: number) => void;
}

// aiSessions polls /ai/sessions so the header can surface a running
// count of Claude threads + briefs across every PR. Threads are
// "live" until the reviewer closes them, so without this view a
// forgotten subprocess sits on the machine indefinitely.
export function createAISessionsStore(opts: AISessionsStoreOptions) {
  const api = opts.client;

  let threads = $state<AISessionThread[]>([]);
  let briefs = $state<AISessionBrief[]>([]);
  let loading = $state(false);
  let errorMsg = $state<string | null>(null);
  let pollHandle: ReturnType<typeof setInterval> | null = null;

  function toThread(raw: {
    id: number;
    mr_id: number;
    repo_owner: string;
    repo_name: string;
    mr_number: number;
    mr_title: string;
    path: string;
    anchor_side: string;
    anchor_line: number;
    created_at: string;
    latest_question_status?: string;
    open_question_count: number;
    has_worktree: boolean;
  }): AISessionThread {
    return {
      id: raw.id,
      mrId: raw.mr_id,
      repoOwner: raw.repo_owner,
      repoName: raw.repo_name,
      mrNumber: raw.mr_number,
      mrTitle: raw.mr_title,
      path: raw.path,
      anchorSide: raw.anchor_side === "LEFT" ? "LEFT" : "RIGHT",
      anchorLine: raw.anchor_line,
      createdAt: raw.created_at,
      latestQuestionStatus: raw.latest_question_status ?? "",
      openQuestionCount: raw.open_question_count,
      hasWorktree: raw.has_worktree,
    };
  }

  function toBrief(raw: {
    id: number;
    mr_id: number;
    repo_owner: string;
    repo_name: string;
    mr_number: number;
    mr_title: string;
    status: string;
    depth: string;
    created_at: string;
    started_at?: string;
  }): AISessionBrief {
    return {
      id: raw.id,
      mrId: raw.mr_id,
      repoOwner: raw.repo_owner,
      repoName: raw.repo_name,
      mrNumber: raw.mr_number,
      mrTitle: raw.mr_title,
      status: raw.status,
      depth: raw.depth,
      createdAt: raw.created_at,
      startedAt: raw.started_at ?? "",
    };
  }

  async function load(): Promise<void> {
    loading = true;
    try {
      const { data, error } = await api.GET("/ai/sessions", {});
      if (error || !data) {
        errorMsg =
          (error as { detail?: string; title?: string })?.detail ??
          (error as { detail?: string; title?: string })?.title ??
          "Failed to load AI sessions";
        return;
      }
      threads = (data.threads ?? []).map(toThread);
      briefs = (data.briefs ?? []).map(toBrief);
      errorMsg = null;
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : String(err);
    } finally {
      loading = false;
    }
  }

  async function closeThread(t: AISessionThread): Promise<void> {
    errorMsg = null;
    const { error } = await api.DELETE(
      "/repos/{owner}/{name}/pulls/{number}/ai-threads/{thread_id}",
      {
        params: {
          path: {
            owner: t.repoOwner,
            name: t.repoName,
            number: t.mrNumber,
            thread_id: t.id,
          },
        },
      },
    );
    if (error) {
      errorMsg =
        (error as { detail?: string; title?: string })?.detail ??
        (error as { detail?: string; title?: string })?.title ??
        "Failed to close thread";
      return;
    }
    threads = threads.filter((x) => x.id !== t.id);
    opts.onThreadDeleted?.(t.id);
  }

  async function cancelBrief(b: AISessionBrief): Promise<void> {
    errorMsg = null;
    const { error } = await api.DELETE(
      "/repos/{owner}/{name}/pulls/{number}/ai-brief",
      {
        params: {
          path: { owner: b.repoOwner, name: b.repoName, number: b.mrNumber },
        },
      },
    );
    if (error) {
      errorMsg =
        (error as { detail?: string; title?: string })?.detail ??
        (error as { detail?: string; title?: string })?.title ??
        "Failed to cancel brief";
      return;
    }
    briefs = briefs.filter((x) => x.id !== b.id);
  }

  function startPolling(intervalMs = 10_000): void {
    stopPolling();
    void load();
    pollHandle = setInterval(() => void load(), intervalMs);
  }

  function stopPolling(): void {
    if (pollHandle !== null) {
      clearInterval(pollHandle);
      pollHandle = null;
    }
  }

  return {
    getThreads: () => threads,
    getBriefs: () => briefs,
    getTotalCount: () => threads.length + briefs.length,
    getRunningCount: () =>
      threads.filter((t) => t.openQuestionCount > 0).length + briefs.length,
    isLoading: () => loading,
    getError: () => errorMsg,
    load,
    closeThread,
    cancelBrief,
    startPolling,
    stopPolling,
  };
}

export type AISessionsStore = ReturnType<typeof createAISessionsStore>;
