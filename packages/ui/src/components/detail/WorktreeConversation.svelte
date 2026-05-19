<script lang="ts">
  import { onMount, untrack } from "svelte";
  import { getStores } from "../../context.js";
  import { renderMarkdown } from "../../utils/markdown.js";

  // Conversation pane for a local worktree session. Renders the
  // back-and-forth between the reviewer (review_feedback +
  // user_message turns) and Claude (claude_response turns). The
  // textbox at the bottom sends free-text follow-ups; review
  // feedback comes in via the ReviewPanel's Submit button.

  interface Props {
    owner: string;
    name: string;
    number: number;
  }
  const { owner, name, number }: Props = $props();

  const { worktreeSession } = getStores();

  // Load + start polling on mount and whenever the selection
  // changes. Wrapped in untrack so the store's internal writes
  // don't feed back into this effect — same shape as the gotcha
  // we hit in WorktreeDetail's first iteration.
  onMount(() => {
    void worktreeSession.loadSession(owner, name, number);
  });

  $effect(() => {
    const o = owner, n = name, num = number;
    untrack(() => {
      void worktreeSession.loadSession(o, n, num);
    });
  });

  const session = $derived(worktreeSession.getSession());
  const turns = $derived(worktreeSession.getTurns());
  const errorMsg = $derived(worktreeSession.getError());
  const hasRunning = $derived(worktreeSession.hasRunningTurn());

  let composer = $state("");
  let submitting = $state(false);

  async function sendUserMessage(): Promise<void> {
    const body = composer.trim();
    if (!body || submitting) return;
    submitting = true;
    try {
      await worktreeSession.submitTurn(owner, name, number, {
        type: "user_message",
        content: body,
      });
      composer = "";
    } catch {
      // store already captured the error; surface via errorMsg
    } finally {
      submitting = false;
    }
  }

  function handleComposerKeydown(e: KeyboardEvent): void {
    // Cmd/Ctrl+Enter submits — matches Claude Code's textarea
    // convention and lets plain Enter insert a newline so reviewers
    // can write multi-line follow-ups.
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      void sendUserMessage();
    }
  }

  function turnLabel(t: { turn_type: string }): string {
    switch (t.turn_type) {
      case "review_feedback": return "Review feedback";
      case "user_message":    return "You";
      case "claude_response": return "Claude";
      case "state":           return "—";
      default:                return t.turn_type;
    }
  }

  function turnRoleClass(t: { turn_type: string }): string {
    if (t.turn_type === "claude_response") return "claude";
    if (t.turn_type === "state") return "state";
    return "user";
  }

  async function cancelTurn(turnID: number): Promise<void> {
    await worktreeSession.cancelTurn(owner, name, number, turnID);
  }

  let killing = $state(false);
  async function killSession(): Promise<void> {
    if (killing) return;
    if (!confirm("Kill this session? Any in-flight turn will be cancelled. " +
        "Submitting new review feedback will start a fresh session.")) {
      return;
    }
    killing = true;
    try {
      await worktreeSession.killSession(owner, name, number);
    } finally {
      killing = false;
    }
  }
</script>

<div class="conv">
  {#if session}
    <div class="conv__header">
      <span class="conv__header-title">Interactive session</span>
      <span class="conv__header-sub">
        active{hasRunning ? " · Claude is working" : ""}
      </span>
      <button
        type="button"
        class="conv__kill-btn"
        onclick={() => void killSession()}
        disabled={killing}
        title="Stop this Claude session. A new session starts on the next submission."
      >
        {killing ? "Killing…" : "Kill session"}
      </button>
    </div>
  {/if}

  {#if errorMsg}
    <div class="conv__error">{errorMsg}</div>
  {/if}

  <div class="conv__scroll">
    {#if turns.length === 0}
      <div class="conv__empty">
        <h2 class="conv__empty-title">No session yet</h2>
        <p class="conv__empty-copy">
          Submit review feedback on the Review tab, or type a message
          below, to start an interactive Claude session against this
          worktree.
        </p>
      </div>
    {:else}
      {#each turns as t (t.id)}
        <article class="turn turn--{turnRoleClass(t)} turn--{t.status}">
          <header class="turn__header">
            <span class="turn__role">{turnLabel(t)}</span>
            {#if t.status === "running"}
              <span class="turn__status">Thinking…</span>
              <button
                type="button"
                class="turn__cancel"
                onclick={() => void cancelTurn(t.id)}
                title="Cancel this turn"
              >Stop</button>
            {:else if t.status === "queued"}
              <span class="turn__status">Queued</span>
            {:else if t.status === "failed"}
              <span class="turn__status turn__status--error">Failed</span>
            {:else if t.status === "cancelled"}
              <span class="turn__status">Cancelled</span>
            {/if}
            <time class="turn__time">{new Date(t.created_at).toLocaleTimeString()}</time>
          </header>
          {#if t.error}
            <pre class="turn__error">{t.error}</pre>
          {/if}
          {#if t.content}
            <div class="turn__body markdown-body">
              {@html renderMarkdown(t.content, { owner, name })}
            </div>
          {:else if t.status === "running" || t.status === "queued"}
            <div class="turn__body turn__body--muted">
              Claude is working on this — the response will appear when it's ready.
            </div>
          {/if}
        </article>
      {/each}
    {/if}
  </div>

  <form
    class="conv__composer"
    onsubmit={(e) => { e.preventDefault(); void sendUserMessage(); }}
  >
    <textarea
      class="conv__textarea"
      placeholder={hasRunning
        ? "Claude is responding… you can queue another message"
        : "Send a message to Claude (Cmd+Enter to send)"}
      bind:value={composer}
      onkeydown={handleComposerKeydown}
      rows="3"
      disabled={submitting}
    ></textarea>
    <div class="conv__composer-actions">
      {#if session}
        <span class="conv__session-id" title={session.id !== 0 ? `Session #${session.id}` : ""}>
          session #{session.id}
        </span>
      {/if}
      <button
        type="submit"
        class="conv__send-btn"
        disabled={submitting || !composer.trim()}
      >
        {submitting ? "Sending…" : "Send"}
      </button>
    </div>
  </form>
</div>

<style>
  .conv {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    background: var(--bg-canvas);
  }

  .conv__header {
    display: flex;
    align-items: baseline;
    gap: 10px;
    padding: 10px 20px;
    border-bottom: 1px solid var(--border-muted);
    background: var(--bg-surface);
    flex-shrink: 0;
  }

  .conv__header-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .conv__header-sub {
    font-size: 11px;
    color: var(--text-muted);
    font-family: var(--font-mono);
  }

  .conv__kill-btn {
    margin-left: auto;
    font-size: 11px;
    padding: 3px 10px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-surface);
    color: var(--text-secondary);
    cursor: pointer;
  }

  .conv__kill-btn:hover:not(:disabled) {
    color: var(--accent-red);
    border-color: color-mix(in srgb, var(--accent-red) 50%, var(--border-muted));
  }

  .conv__kill-btn:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .conv__error {
    margin: 8px 16px 0;
    padding: 6px 10px;
    border-radius: var(--radius-sm);
    background: color-mix(in srgb, var(--accent-red) 10%, transparent);
    color: var(--accent-red);
    font-size: 12px;
  }

  .conv__scroll {
    flex: 1;
    overflow-y: auto;
    padding: 16px 20px;
  }

  .conv__empty {
    max-width: 520px;
    margin: 40px auto;
    padding: 20px 24px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-md);
    background: var(--bg-surface);
  }

  .conv__empty-title {
    margin: 0 0 8px;
    font-size: 14px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .conv__empty-copy {
    margin: 0;
    font-size: 13px;
    line-height: 1.5;
    color: var(--text-secondary);
  }

  .turn {
    margin-bottom: 16px;
    padding: 12px 16px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-md);
    background: var(--bg-surface);
  }

  .turn--claude {
    background: color-mix(in srgb, var(--accent-blue) 4%, var(--bg-surface));
    border-color: color-mix(in srgb, var(--accent-blue) 24%, var(--border-muted));
  }

  .turn__header {
    display: flex;
    align-items: baseline;
    gap: 10px;
    margin-bottom: 8px;
  }

  .turn__role {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .turn__status {
    font-size: 11px;
    color: var(--text-muted);
    font-style: italic;
  }

  .turn__status--error {
    color: var(--accent-red);
    font-style: normal;
    font-weight: 600;
  }

  .turn__cancel {
    font-size: 10px;
    padding: 1px 6px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-surface);
    color: var(--text-secondary);
    cursor: pointer;
  }

  .turn__cancel:hover {
    background: var(--bg-surface-hover);
  }

  .turn__time {
    margin-left: auto;
    font-size: 10px;
    color: var(--text-muted);
  }

  .turn__body {
    font-size: 13px;
    line-height: 1.5;
    color: var(--text-primary);
  }

  .turn__body--muted {
    color: var(--text-muted);
    font-style: italic;
  }

  .turn__error {
    margin: 0 0 6px;
    padding: 8px 10px;
    border-radius: var(--radius-sm);
    background: color-mix(in srgb, var(--accent-red) 8%, transparent);
    color: var(--accent-red);
    font-size: 11px;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .conv__composer {
    border-top: 1px solid var(--border-default);
    padding: 12px 16px;
    background: var(--bg-surface);
    flex-shrink: 0;
  }

  .conv__textarea {
    width: 100%;
    resize: vertical;
    font-size: 13px;
    padding: 8px 10px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-inset);
    color: var(--text-primary);
    font-family: inherit;
  }

  .conv__textarea:focus {
    border-color: var(--accent-blue);
    outline: none;
  }

  .conv__composer-actions {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 8px;
  }

  .conv__session-id {
    font-size: 11px;
    color: var(--text-muted);
    font-family: var(--font-mono);
  }

  .conv__send-btn {
    margin-left: auto;
    padding: 5px 14px;
    font-size: 12px;
    font-weight: 600;
    border: 1px solid var(--accent-blue);
    border-radius: var(--radius-sm);
    background: var(--accent-blue);
    color: #fff;
    cursor: pointer;
  }

  .conv__send-btn:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
