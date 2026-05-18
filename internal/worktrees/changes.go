package worktrees

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ChangedFile is one entry in a worktree's current change set —
// what `git diff HEAD` would report. Additions/Deletions are 0 for
// binary files; IsBinary disambiguates.
type ChangedFile struct {
	Path       string `json:"path"`
	OldPath    string `json:"old_path"`
	Status     string `json:"status"` // added | modified | deleted | renamed | copied
	IsBinary   bool   `json:"is_binary"`
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
}

// ListChangedFiles returns the files modified in the worktree
// relative to its HEAD — committed changes are NOT included; only
// the live, uncommitted state (staged + unstaged + untracked-as-far-as-
// git-knows). This is the "what's currently in flight here" view.
//
// Two git calls: --raw -z for status letters and rename detection,
// --numstat -z for line counts. Results are cross-referenced by
// path; entries present in --raw but missing from --numstat (e.g.
// binary files) are returned with IsBinary set.
//
// Returns a nil slice when the worktree is clean.
func ListChangedFiles(ctx context.Context, worktreePath string) ([]ChangedFile, error) {
	if worktreePath == "" {
		return nil, fmt.Errorf("worktreePath is required")
	}
	rawOut, err := gitCmd(ctx, worktreePath,
		"diff", "HEAD", "--raw", "-z", "-M", "-C", "--find-copies-harder",
	)
	if err != nil {
		return nil, fmt.Errorf("git diff --raw: %w", err)
	}
	files := parseRawZ(rawOut)

	numstatOut, err := gitCmd(ctx, worktreePath,
		"diff", "HEAD", "--numstat", "-z",
	)
	if err != nil {
		return nil, fmt.Errorf("git diff --numstat: %w", err)
	}
	counts := parseNumstatZ(numstatOut)

	for i := range files {
		c, ok := counts[files[i].Path]
		if !ok {
			c, ok = counts[files[i].OldPath]
		}
		if ok {
			files[i].Additions = c.adds
			files[i].Deletions = c.dels
			files[i].IsBinary = c.binary
		} else {
			// Path missing from numstat is unusual; treat as binary.
			files[i].IsBinary = true
		}
	}
	return files, nil
}

func gitCmd(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

type numstatRow struct {
	adds, dels int
	binary     bool
}

// parseNumstatZ parses `git diff --numstat -z` output. Format:
//
//	<adds>\t<dels>\t<path>\0
//
// Renames use a 3-NUL form: <adds>\t<dels>\t\0<oldpath>\0<newpath>\0.
// We key by new path so callers correlate against --raw results.
func parseNumstatZ(data []byte) map[string]numstatRow {
	out := make(map[string]numstatRow)
	parts := bytes.Split(data, []byte{0})
	for i := 0; i < len(parts); i++ {
		entry := string(parts[i])
		if entry == "" {
			continue
		}
		fields := strings.SplitN(entry, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		adds, addsRaw := fields[0], fields[0]
		dels, delsRaw := fields[1], fields[1]
		path := fields[2]
		row := numstatRow{}
		if addsRaw == "-" && delsRaw == "-" {
			row.binary = true
		} else {
			row.adds, _ = strconv.Atoi(adds)
			row.dels, _ = strconv.Atoi(dels)
		}
		if path == "" {
			// Rename: next two NULs are oldpath, newpath.
			if i+2 >= len(parts) {
				break
			}
			_ = parts[i+1] // oldpath, not needed for keying
			newPath := string(parts[i+2])
			out[newPath] = row
			i += 2
			continue
		}
		out[path] = row
	}
	return out
}

// parseRawZ parses `git diff --raw -z` output. Each entry header line
// begins with ':' and is followed by one or two NUL-terminated paths
// (two for renames/copies). Format:
//
//	:oldmode newmode oldhash newhash status\0path\0
//	:...                          R100\0oldpath\0newpath\0
func parseRawZ(data []byte) []ChangedFile {
	parts := bytes.Split(data, []byte{0})
	var files []ChangedFile

	i := 0
	for i < len(parts) {
		part := string(parts[i])
		if !strings.HasPrefix(part, ":") {
			i++
			continue
		}
		fields := strings.Fields(part)
		if len(fields) < 5 {
			i++
			continue
		}
		statusRaw := fields[4]
		status, isRename := rawStatusToString(statusRaw)

		i++
		if i >= len(parts) {
			break
		}
		path := string(parts[i])

		var oldPath string
		if isRename {
			oldPath = path
			i++
			if i >= len(parts) {
				break
			}
			path = string(parts[i])
		}
		if oldPath == "" {
			oldPath = path
		}

		files = append(files, ChangedFile{
			Path:    path,
			OldPath: oldPath,
			Status:  status,
		})
		i++
	}
	return files
}

func rawStatusToString(s string) (status string, isRenameOrCopy bool) {
	if len(s) == 0 {
		return "modified", false
	}
	switch s[0] {
	case 'A':
		return "added", false
	case 'D':
		return "deleted", false
	case 'M':
		return "modified", false
	case 'R':
		return "renamed", true
	case 'C':
		return "copied", true
	case 'T':
		return "modified", false
	default:
		return "modified", false
	}
}
