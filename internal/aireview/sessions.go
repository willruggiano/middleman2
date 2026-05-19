package aireview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"

	"github.com/wesm/middleman/internal/db"
)

// SessionRunner owns interactive Claude sessions bound to local
// worktrees. It's the agent loop the user drives from the Activity
// tab: review-feedback submissions and free-text follow-ups land as
// turns; each turn spawns one `claude -p` subprocess that resumes
// the session via --resume after the first turn.
//
// Compared to the existing Runner (ask threads, briefs, commit
// analyses), SessionRunner:
//
//   - operates against the user's existing worktree, not a
//     provisioned ephemeral one
//   - allows Edit / Write / Bash so Claude can make changes in
//     response to feedback
//   - persists turns to middleman_worktree_session_turns instead of
//     ai_questions / ai_briefs
//   - keys cancellation by turn id rather than question id
//
// Like the Runner, every Claude invocation is one-shot: the runner
// doesn't keep a long-lived subprocess. State continuity is the
// CLI's `--resume <session_id>` feature, fed by the session row.
type SessionRunner struct {
	db SessionDB

	mu      sync.Mutex
	running map[int64]context.CancelFunc // by turn id
}

// SessionDB is the narrow DB surface the SessionRunner needs.
// Exposed as an interface for stubbing in tests.
type SessionDB interface {
	GetWorktreeSession(ctx context.Context, id int64) (db.WorktreeSession, error)
	SetWorktreeSessionClaudeID(ctx context.Context, id int64, claudeSessionID string) error
	AddWorktreeSessionTurn(ctx context.Context, in db.NewWorktreeSessionTurn) (db.WorktreeSessionTurn, error)
	UpdateWorktreeSessionTurnFields(ctx context.Context, id int64, u db.UpdateWorktreeSessionTurn) error
	ListRunningWorktreeSessionTurns(ctx context.Context) ([]db.WorktreeSessionTurn, error)
}

// NewSessionRunner builds a runner backed by the given DB.
func NewSessionRunner(database SessionDB) *SessionRunner {
	return &SessionRunner{
		db:      database,
		running: make(map[int64]context.CancelFunc),
	}
}

// ReconcileOnStartup marks any leftover queued/running session
// turns as failed. Their subprocesses didn't survive the middleman
// restart; without this the UI would show them as in-flight
// forever. The session row itself is left alive — the user can
// submit a new turn against the same Claude session via --resume.
func (r *SessionRunner) ReconcileOnStartup(ctx context.Context) error {
	orphan, err := r.db.ListRunningWorktreeSessionTurns(ctx)
	if err != nil {
		return fmt.Errorf("list running session turns: %w", err)
	}
	failed := "failed"
	msg := "interrupted by middleman restart"
	for _, t := range orphan {
		_ = r.db.UpdateWorktreeSessionTurnFields(ctx, t.ID, db.UpdateWorktreeSessionTurn{
			Status:   &failed,
			Error:    &msg,
			ClearPID: true,
		})
	}
	return nil
}

// SubmitTurnInput packages a user-driven turn submission.
type SubmitTurnInput struct {
	SessionID    int64
	WorktreePath string // cwd for the Claude subprocess
	Branch       string // for context priming
	BaseRef      string
	BaseSHA      string
	HeadSHA      string
	// UserTurnType is "review_feedback" or "user_message".
	UserTurnType    string
	UserTurnContent string
	// UserTurnMetadataJSON is opaque metadata stored alongside the
	// user turn (e.g. linked draft comment ids for review_feedback).
	UserTurnMetadataJSON string
	// IsFirstTurn signals that this is the first turn in the
	// session — the runner primes the prompt with worktree context
	// instead of relying on --resume.
	IsFirstTurn bool
}

// SubmitResult bundles the two new turn rows created by SubmitTurn.
type SubmitResult struct {
	UserTurn     db.WorktreeSessionTurn
	ResponseTurn db.WorktreeSessionTurn
}

// SubmitTurn inserts the user turn + a queued claude_response turn,
// then spawns Claude in the background. Returns immediately with
// both rows hydrated. Callers poll/subscribe to the response turn
// for status transitions.
func (r *SessionRunner) SubmitTurn(
	ctx context.Context, in SubmitTurnInput,
) (SubmitResult, error) {
	if in.SessionID == 0 {
		return SubmitResult{}, errors.New("session id required")
	}
	if in.WorktreePath == "" {
		return SubmitResult{}, errors.New("worktree path required")
	}
	if in.UserTurnType != "review_feedback" && in.UserTurnType != "user_message" {
		return SubmitResult{}, fmt.Errorf("unsupported user turn type %q", in.UserTurnType)
	}

	userTurn, err := r.db.AddWorktreeSessionTurn(ctx, db.NewWorktreeSessionTurn{
		SessionID:    in.SessionID,
		TurnType:     in.UserTurnType,
		Content:      in.UserTurnContent,
		Status:       "done",
		MetadataJSON: in.UserTurnMetadataJSON,
	})
	if err != nil {
		return SubmitResult{}, fmt.Errorf("insert user turn: %w", err)
	}

	respTurn, err := r.db.AddWorktreeSessionTurn(ctx, db.NewWorktreeSessionTurn{
		SessionID: in.SessionID,
		TurnType:  "claude_response",
		Status:    "queued",
	})
	if err != nil {
		return SubmitResult{}, fmt.Errorf("insert response turn: %w", err)
	}

	r.spawnTurn(in, respTurn)
	return SubmitResult{UserTurn: userTurn, ResponseTurn: respTurn}, nil
}

// CancelTurn kills any in-flight subprocess for the response turn
// and marks the row cancelled.
func (r *SessionRunner) CancelTurn(ctx context.Context, turnID int64) error {
	r.mu.Lock()
	cancel, ok := r.running[turnID]
	r.mu.Unlock()
	if ok {
		cancel()
	}
	cancelled := "cancelled"
	return r.db.UpdateWorktreeSessionTurnFields(ctx, turnID, db.UpdateWorktreeSessionTurn{
		Status:   &cancelled,
		ClearPID: true,
	})
}

func (r *SessionRunner) spawnTurn(in SubmitTurnInput, respTurn db.WorktreeSessionTurn) {
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.running[respTurn.ID] = cancel
	r.mu.Unlock()

	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.running, respTurn.ID)
			r.mu.Unlock()
			cancel()
		}()
		r.runTurn(ctx, in, respTurn)
	}()
}

func (r *SessionRunner) runTurn(
	ctx context.Context, in SubmitTurnInput, respTurn db.WorktreeSessionTurn,
) {
	prompt := buildSessionPrompt(in)

	// Pull the current session row to get the most recent
	// claude_session_id — earlier turns in the same session may have
	// already populated it.
	sess, err := r.db.GetWorktreeSession(ctx, in.SessionID)
	if err != nil {
		r.markFailed(ctx, respTurn.ID, "load session: "+err.Error())
		return
	}

	args := []string{
		"-p", prompt,
		"--output-format", "json",
		"--permission-mode", "bypassPermissions",
		"--allowedTools", "Read,Glob,Grep,Edit,Write,MultiEdit,Bash",
	}
	if sess.ClaudeSessionID != "" {
		args = append(args, "--resume", sess.ClaudeSessionID)
	}

	cmd := exec.CommandContext(ctx, claudeBinary, args...)
	cmd.Dir = in.WorktreePath
	setPgid(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.markFailed(ctx, respTurn.ID, "stdout pipe: "+err.Error())
		return
	}
	stderrBuf := &strings.Builder{}
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		r.markFailed(ctx, respTurn.ID, "start claude: "+err.Error())
		return
	}
	pid := cmd.Process.Pid
	running := "running"
	if err := r.db.UpdateWorktreeSessionTurnFields(ctx, respTurn.ID, db.UpdateWorktreeSessionTurn{
		Status: &running,
		PID:    &pid,
	}); err != nil {
		slog.Warn("mark session turn running failed", "turn_id", respTurn.ID, "err", err)
	}

	raw, readErr := readAll(stdout)
	waitErr := cmd.Wait()

	if ctx.Err() != nil {
		// CancelTurn flipped the row to cancelled; don't overwrite.
		return
	}
	if waitErr != nil {
		msg := fmt.Sprintf("claude exited: %v", waitErr)
		if stderrBuf.Len() > 0 {
			msg += "\n" + strings.TrimSpace(stderrBuf.String())
		}
		if readErr != nil {
			msg += "\nread: " + readErr.Error()
		}
		r.markFailed(ctx, respTurn.ID, msg)
		return
	}

	result, err := parseClaudeResult(raw)
	if err != nil {
		r.markFailed(ctx, respTurn.ID, fmt.Sprintf("parse claude output: %v\noutput: %s", err, snippet(raw, 400)))
		return
	}

	// Persist the session id on first turn so follow-ups can --resume.
	if sess.ClaudeSessionID == "" && result.SessionID != "" {
		if err := r.db.SetWorktreeSessionClaudeID(ctx, in.SessionID, result.SessionID); err != nil {
			slog.Warn("save claude session id failed", "session_id", in.SessionID, "err", err)
		}
	}

	rawStr := string(raw)
	done := "done"
	if err := r.db.UpdateWorktreeSessionTurnFields(ctx, respTurn.ID, db.UpdateWorktreeSessionTurn{
		Status:   &done,
		Content:  &result.Text,
		RawJSON:  &rawStr,
		ClearPID: true,
	}); err != nil {
		slog.Warn("mark session turn done failed", "turn_id", respTurn.ID, "err", err)
	}
}

func (r *SessionRunner) markFailed(ctx context.Context, turnID int64, msg string) {
	failed := "failed"
	if err := r.db.UpdateWorktreeSessionTurnFields(ctx, turnID, db.UpdateWorktreeSessionTurn{
		Status:   &failed,
		Error:    &msg,
		ClearPID: true,
	}); err != nil {
		slog.Warn("mark session turn failed failed", "turn_id", turnID, "err", err)
	}
}

// buildSessionPrompt assembles the prompt sent on each turn. The
// first turn primes Claude with worktree context (where it is,
// what branch, what base, what's middleman expecting). Subsequent
// turns rely on --resume so the model keeps that context already
// — we only send the new user message.
func buildSessionPrompt(in SubmitTurnInput) string {
	if !in.IsFirstTurn {
		return in.UserTurnContent
	}
	var b strings.Builder
	b.WriteString(
		"You are operating inside a git worktree that middleman is " +
			"presenting to a reviewer as a draft PR. ",
	)
	b.WriteString(
		"You have full Read / Edit / Write / Bash access to this worktree; " +
			"act directly to address the reviewer's feedback rather than describing what you'd do. ",
	)
	b.WriteString(
		"When you finish, summarize what changed and where (file paths + a one-line description per change). ",
	)
	b.WriteString(
		"You can use `git log`, `git diff`, etc. to ground yourself in the commit history. " +
			"Don't push, don't open PRs, don't touch remotes.\n\n",
	)
	b.WriteString("Worktree path: ")
	b.WriteString(in.WorktreePath)
	b.WriteString("\n")
	if in.Branch != "" {
		b.WriteString("Branch: ")
		b.WriteString(in.Branch)
		b.WriteString("\n")
	}
	if in.BaseRef != "" {
		b.WriteString("Base: ")
		b.WriteString(in.BaseRef)
		if in.BaseSHA != "" {
			b.WriteString(" (")
			b.WriteString(shortSHA(in.BaseSHA))
			b.WriteString(")")
		}
		b.WriteString("\n")
	}
	if in.HeadSHA != "" {
		b.WriteString("HEAD: ")
		b.WriteString(shortSHA(in.HeadSHA))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if in.UserTurnType == "review_feedback" {
		b.WriteString("The reviewer has submitted the following feedback. " +
			"Read the comments, look at the relevant files, and make the requested changes.\n\n")
	}
	b.WriteString(in.UserTurnContent)
	return b.String()
}

func shortSHA(sha string) string {
	if len(sha) < 7 {
		return sha
	}
	return sha[:7]
}

// (encoding/json import is reserved for future tool-call rendering;
// the current implementation only round-trips raw JSON as a string.)
var _ = json.RawMessage(nil)
