package gitclone

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// emptyTreeSHA is git's well-known SHA for an empty tree object.
const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// Commit holds metadata for a single commit in a PR's history.
type Commit struct {
	SHA        string
	AuthorName string
	AuthoredAt time.Time
	Message    string // subject (first line) only
	Body       string // remainder of the commit message after the subject, trimmed
}

// ListCommits returns commits between mergeBase and headSHA, newest first,
// following only the first-parent chain. If mergeBase is the empty tree
// sentinel (parentless root), all commits up to headSHA are returned.
func (m *Manager) ListCommits(
	ctx context.Context,
	host, owner, name, mergeBase, headSHA string,
) ([]Commit, error) {
	dir := m.ClonePath(host, owner, name)

	// -z terminates each log entry with NUL (instead of newline), which
	// lets the raw body (%B) safely contain newlines. We use NUL between
	// fields as well, so the output is a flat stream of NUL-separated
	// fields consumed in groups of 4.
	args := []string{"log", "-z", "--first-parent", "--format=%H%x00%an%x00%aI%x00%B"}
	if mergeBase == emptyTreeSHA {
		// Empty tree is not a commit — list all ancestors of head.
		args = append(args, headSHA)
	} else {
		args = append(args, mergeBase+".."+headSHA)
	}

	out, err := m.git(ctx, host, dir, args...)
	if err != nil {
		return nil, fmt.Errorf("list commits: %w", err)
	}

	s := strings.TrimSuffix(string(out), "\x00")
	if s == "" {
		return nil, nil
	}
	fields := strings.Split(s, "\x00")
	if len(fields)%4 != 0 {
		return nil, fmt.Errorf("unexpected git log field count: %d", len(fields))
	}

	commits := make([]Commit, 0, len(fields)/4)
	for i := 0; i < len(fields); i += 4 {
		t, err := time.Parse(time.RFC3339, fields[i+2])
		if err != nil {
			return nil, fmt.Errorf("parse commit date %q: %w", fields[i+2], err)
		}
		// %B includes a trailing newline; drop it so the body isn't padded.
		msg := strings.TrimRight(fields[i+3], "\n")
		subject, body, _ := strings.Cut(msg, "\n")
		commits = append(commits, Commit{
			SHA:        fields[i],
			AuthorName: fields[i+1],
			AuthoredAt: t,
			Message:    subject,
			Body:       strings.TrimSpace(body),
		})
	}
	return commits, nil
}

// ParentOf returns the first parent SHA of the given commit.
// For a parentless (root) commit it returns the empty tree sentinel.
// The caller must ensure sha exists in the clone; any failure here
// is a genuine server-side error, not a client-input issue.
func (m *Manager) ParentOf(
	ctx context.Context,
	host, owner, name, sha string,
) (string, error) {
	dir := m.ClonePath(host, owner, name)
	out, err := m.git(ctx, host, dir,
		"rev-list", "--parents", "-n", "1", sha,
	)
	if err != nil {
		return "", fmt.Errorf("parent of %s: %w", sha, err)
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) == 0 {
		return "", fmt.Errorf("parent of %s: empty rev-list output", sha)
	}
	if len(fields) == 1 {
		// Parentless commit — diff against the empty tree.
		return emptyTreeSHA, nil
	}
	return fields[1], nil
}
