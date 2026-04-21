import type { components } from "../api/generated/schema.js";

// Structural types from the generated OpenAPI schema.
export type AIThread = components["schemas"]["AiThreadResponse"];
export type AIQuestion = components["schemas"]["AiQuestionResponse"];

export interface AIStoreOptions {
  getBasePath?: () => string;
}

export function createAIStore(opts?: AIStoreOptions) {
  const getBasePath = opts?.getBasePath ?? (() => "/");

  // Active PR context. Polling and the store contents are keyed here.
  let owner = $state("");
  let name = $state("");
  let number = $state(0);

  let threads = $state<AIThread[]>([]);
  let questions = $state<AIQuestion[]>([]);
  let loading = $state(false);
  let pollHandle: ReturnType<typeof setInterval> | null = null;

  function prefix(): string {
    return `${getBasePath()}api/v1/repos/${encodeURIComponent(owner)}/${encodeURIComponent(name)}/pulls/${number}`;
  }

  // --- reads ---

  function getThreadsForFile(path: string): AIThread[] {
    return threads.filter((t) => t.path === path);
  }

  function getThreadsAtAnchor(path: string, line: number, side: "LEFT" | "RIGHT"): AIThread[] {
    return threads.filter(
      (t) => t.path === path && t.anchor_line === line && t.anchor_side === side,
    );
  }

  function getQuestionsForThread(threadID: number): AIQuestion[] {
    return questions
      .filter((q) => q.thread_id === threadID)
      .sort((a, b) => a.id - b.id);
  }

  function hasInFlightQuestions(): boolean {
    return questions.some((q) => q.status === "queued" || q.status === "running");
  }

  function all(): { threads: AIThread[]; questions: AIQuestion[] } {
    return { threads, questions };
  }

  // --- writes ---

  async function refresh(): Promise<void> {
    if (!owner) return;
    loading = true;
    try {
      const res = await fetch(`${prefix()}/ai-threads`);
      if (!res.ok) return;
      const data = (await res.json()) as {
        threads: AIThread[] | null;
        questions: AIQuestion[] | null;
      };
      threads = data.threads ?? [];
      questions = data.questions ?? [];
    } catch {
      /* swallow; next poll will retry */
    } finally {
      loading = false;
    }
  }

  async function createThread(input: {
    path: string;
    anchor_side: "LEFT" | "RIGHT";
    anchor_line: number;
    hunk_start_line?: number;
    hunk_end_line?: number;
    hunk_text?: string;
    selection_text?: string;
    commit_sha: string;
    question: string;
    prompt_context?: string;
  }): Promise<{ thread: AIThread; question: AIQuestion } | null> {
    try {
      const res = await fetch(`${prefix()}/ai-threads`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(input),
      });
      if (!res.ok) return null;
      const data = (await res.json()) as { thread: AIThread; question: AIQuestion };
      threads = [...threads, data.thread];
      questions = [...questions, data.question];
      return data;
    } catch {
      return null;
    }
  }

  async function addFollowUp(
    threadID: number,
    question: string,
  ): Promise<AIQuestion | null> {
    try {
      const res = await fetch(`${prefix()}/ai-threads/${threadID}/questions`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ question }),
      });
      if (!res.ok) return null;
      const q = (await res.json()) as AIQuestion;
      questions = [...questions, q];
      return q;
    } catch {
      return null;
    }
  }

  async function deleteThread(threadID: number): Promise<void> {
    try {
      await fetch(`${prefix()}/ai-threads/${threadID}`, { method: "DELETE" });
    } finally {
      threads = threads.filter((t) => t.id !== threadID);
      questions = questions.filter((q) => q.thread_id !== threadID);
    }
  }

  async function deleteQuestion(threadID: number, questionID: number): Promise<void> {
    try {
      await fetch(`${prefix()}/ai-threads/${threadID}/questions/${questionID}`, {
        method: "DELETE",
      });
    } finally {
      questions = questions.filter((q) => q.id !== questionID);
    }
  }

  // --- lifecycle ---

  function start(o: string, n: string, num: number): void {
    const changed = o !== owner || n !== name || num !== number;
    owner = o;
    name = n;
    number = num;
    if (changed) {
      threads = [];
      questions = [];
    }
    void refresh();
    stopPolling();
    pollHandle = setInterval(() => {
      // Throttle polling when nothing is in-flight; still refresh at a
      // slower cadence to pick up foreign mutations (e.g. delete from
      // another tab), but don't hammer the server while the reviewer is
      // just reading.
      if (hasInFlightQuestions()) {
        void refresh();
      }
    }, 2000);
  }

  function stopPolling(): void {
    if (pollHandle !== null) {
      clearInterval(pollHandle);
      pollHandle = null;
    }
  }

  function stop(): void {
    stopPolling();
    owner = "";
    name = "";
    number = 0;
    threads = [];
    questions = [];
  }

  return {
    getThreadsForFile,
    getThreadsAtAnchor,
    getQuestionsForThread,
    hasInFlightQuestions,
    all,
    refresh,
    createThread,
    addFollowUp,
    deleteThread,
    deleteQuestion,
    start,
    stop,
  };
}

export type AIStore = ReturnType<typeof createAIStore>;
