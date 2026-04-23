package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AuthorGroup is a reviewer-local named list of GitHub logins. The
// dashboard uses it to filter the PR list to "people I care about
// right now" without having to re-pick the same 10 usernames out of
// a big dropdown every time.
type AuthorGroup struct {
	ID        int64
	Name      string
	Members   []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ErrAuthorGroupNameTaken is returned by CreateAuthorGroup /
// UpdateAuthorGroup when the requested name already belongs to a
// different group. The API layer translates this into a 409.
var ErrAuthorGroupNameTaken = errors.New("author group name already in use")

// normalizeGroupMembers trims whitespace, drops empties, and
// deduplicates case-insensitively while preserving the first
// occurrence's casing. GitHub logins are case-insensitive at
// lookup but we keep the originally-entered form for display.
func normalizeGroupMembers(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, m := range in {
		trimmed := strings.TrimSpace(m)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func (d *DB) ListAuthorGroups(ctx context.Context) ([]AuthorGroup, error) {
	rows, err := d.ro.QueryContext(ctx,
		`SELECT id, name, members_json, created_at, updated_at
		   FROM middleman_author_groups
		  ORDER BY name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, fmt.Errorf("list author groups: %w", err)
	}
	defer rows.Close()

	var out []AuthorGroup
	for rows.Next() {
		g, err := scanAuthorGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (d *DB) GetAuthorGroup(ctx context.Context, id int64) (AuthorGroup, error) {
	return scanAuthorGroup(d.ro.QueryRowContext(ctx,
		`SELECT id, name, members_json, created_at, updated_at
		   FROM middleman_author_groups WHERE id = ?`, id))
}

func (d *DB) CreateAuthorGroup(ctx context.Context, name string, members []string) (AuthorGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return AuthorGroup{}, fmt.Errorf("author group name is required")
	}
	members = normalizeGroupMembers(members)
	raw, err := json.Marshal(members)
	if err != nil {
		return AuthorGroup{}, fmt.Errorf("marshal members: %w", err)
	}
	res, err := d.rw.ExecContext(ctx,
		`INSERT INTO middleman_author_groups (name, members_json) VALUES (?, ?)`,
		name, string(raw),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return AuthorGroup{}, ErrAuthorGroupNameTaken
		}
		return AuthorGroup{}, fmt.Errorf("insert author group: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return AuthorGroup{}, err
	}
	return d.GetAuthorGroup(ctx, id)
}

func (d *DB) UpdateAuthorGroup(ctx context.Context, id int64, name string, members []string) (AuthorGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return AuthorGroup{}, fmt.Errorf("author group name is required")
	}
	members = normalizeGroupMembers(members)
	raw, err := json.Marshal(members)
	if err != nil {
		return AuthorGroup{}, fmt.Errorf("marshal members: %w", err)
	}
	res, err := d.rw.ExecContext(ctx,
		`UPDATE middleman_author_groups
		    SET name = ?, members_json = ?, updated_at = datetime('now')
		  WHERE id = ?`,
		name, string(raw), id,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return AuthorGroup{}, ErrAuthorGroupNameTaken
		}
		return AuthorGroup{}, fmt.Errorf("update author group: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return AuthorGroup{}, err
	}
	if n == 0 {
		return AuthorGroup{}, sql.ErrNoRows
	}
	return d.GetAuthorGroup(ctx, id)
}

func (d *DB) DeleteAuthorGroup(ctx context.Context, id int64) error {
	_, err := d.rw.ExecContext(ctx,
		`DELETE FROM middleman_author_groups WHERE id = ?`, id)
	return err
}

func scanAuthorGroup(row scanner) (AuthorGroup, error) {
	var g AuthorGroup
	var membersRaw string
	if err := row.Scan(&g.ID, &g.Name, &membersRaw, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return AuthorGroup{}, err
	}
	if err := json.Unmarshal([]byte(membersRaw), &g.Members); err != nil {
		return AuthorGroup{}, fmt.Errorf("unmarshal members for group %d: %w", g.ID, err)
	}
	if g.Members == nil {
		g.Members = []string{}
	}
	return g, nil
}
