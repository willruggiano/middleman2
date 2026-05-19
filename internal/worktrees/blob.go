package worktrees

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotFound signals that the requested blob does not exist at the
// given SHA/path, or the working-tree file is missing from disk.
// Callers can distinguish this from generic git failures to return
// 404 rather than 502.
var ErrNotFound = errors.New("worktree blob not found")

// Blob returns the file content at the given SHA and path within
// the worktree. The special WorkingTreeSentinel SHA reads the file
// straight off disk so callers can fetch uncommitted content
// alongside historical revisions through one entry point.
func Blob(
	ctx context.Context, worktreePath, sha, path string,
) ([]byte, error) {
	if sha == WorkingTreeSentinel {
		full := filepath.Join(worktreePath, path)
		raw, err := os.ReadFile(full)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
			}
			return nil, fmt.Errorf("read working-tree file %s: %w", path, err)
		}
		return raw, nil
	}
	out, err := gitCmd(ctx, worktreePath, "cat-file", "-p", sha+":"+path)
	if err != nil {
		// `git cat-file` doesn't expose a stable not-found exit code,
		// so we string-match its stderr. Both forms are emitted in
		// practice; the lowercase variant is the message for a path
		// missing under a known SHA, the capitalized one for an
		// unparseable object reference.
		msg := err.Error()
		if strings.Contains(msg, "does not exist") ||
			strings.Contains(msg, "Not a valid object name") {
			return nil, fmt.Errorf("%w: %s:%s", ErrNotFound, sha, path)
		}
		return nil, fmt.Errorf("cat-file %s:%s: %w", sha, path, err)
	}
	return out, nil
}
