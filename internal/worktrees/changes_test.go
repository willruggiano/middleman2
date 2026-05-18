package worktrees

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListChangedFilesEmptyWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	require := require.New(t)
	ctx := context.Background()

	dir := t.TempDir()
	runGitT(t, "", "init", "--initial-branch=main", dir)
	runGitT(t, dir, "config", "user.email", "test@example.com")
	runGitT(t, dir, "config", "user.name", "Test")
	runGitT(t, dir, "commit", "--allow-empty", "-m", "init")

	got, err := ListChangedFiles(ctx, dir)
	require.NoError(err)
	require.Empty(got)
}

func TestListChangedFilesPicksUpAddedAndModified(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	require := require.New(t)
	assert := assert.New(t)
	ctx := context.Background()

	dir := t.TempDir()
	runGitT(t, "", "init", "--initial-branch=main", dir)
	runGitT(t, dir, "config", "user.email", "test@example.com")
	runGitT(t, dir, "config", "user.name", "Test")
	require.NoError(os.WriteFile(filepath.Join(dir, "kept.txt"), []byte("alpha\nbeta\n"), 0o644))
	runGitT(t, dir, "add", "kept.txt")
	runGitT(t, dir, "commit", "-m", "init")

	// Modify the committed file (uncommitted change).
	require.NoError(os.WriteFile(filepath.Join(dir, "kept.txt"), []byte("alpha\nbeta\ngamma\n"), 0o644))
	// Add a new file (uncommitted).
	require.NoError(os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello\n"), 0o644))
	runGitT(t, dir, "add", "new.txt")

	got, err := ListChangedFiles(ctx, dir)
	require.NoError(err)
	require.Len(got, 2)

	byPath := map[string]ChangedFile{got[0].Path: got[0], got[1].Path: got[1]}

	mod := byPath["kept.txt"]
	assert.Equal("modified", mod.Status)
	assert.Equal(1, mod.Additions)
	assert.Equal(0, mod.Deletions)
	assert.False(mod.IsBinary)

	added := byPath["new.txt"]
	assert.Equal("added", added.Status)
	assert.Equal(1, added.Additions)
	assert.Equal(0, added.Deletions)
}

func TestListChangedFilesDetectsDelete(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	require := require.New(t)
	assert := assert.New(t)
	ctx := context.Background()

	dir := t.TempDir()
	runGitT(t, "", "init", "--initial-branch=main", dir)
	runGitT(t, dir, "config", "user.email", "test@example.com")
	runGitT(t, dir, "config", "user.name", "Test")
	require.NoError(os.WriteFile(filepath.Join(dir, "doomed.txt"), []byte("alpha\nbeta\n"), 0o644))
	runGitT(t, dir, "add", "doomed.txt")
	runGitT(t, dir, "commit", "-m", "init")
	require.NoError(os.Remove(filepath.Join(dir, "doomed.txt")))
	runGitT(t, dir, "add", "-A", "doomed.txt")

	got, err := ListChangedFiles(ctx, dir)
	require.NoError(err)
	require.Len(got, 1)
	assert.Equal("doomed.txt", got[0].Path)
	assert.Equal("deleted", got[0].Status)
	assert.Equal(0, got[0].Additions)
	assert.Equal(2, got[0].Deletions)
}

func runGitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, string(out))
}
