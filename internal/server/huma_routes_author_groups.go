package server

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/wesm/middleman/internal/db"
)

// authorGroupMaxMembers caps how many logins a single group can
// hold. Guarded at the API boundary so the DB never grows an
// unbounded JSON blob.
const authorGroupMaxMembers = 500

type listAuthorGroupsOutput struct {
	Body authorGroupsResponse
}

type createAuthorGroupInput struct {
	Body struct {
		Name    string   `json:"name"    doc:"Display name; must be unique"`
		Members []string `json:"members" doc:"GitHub logins that belong to this group"`
	}
}

type authorGroupPathInput struct {
	ID int64 `path:"id"`
}

type updateAuthorGroupInput struct {
	ID   int64 `path:"id"`
	Body struct {
		Name    string   `json:"name"`
		Members []string `json:"members"`
	}
}

type authorGroupOutput struct {
	Body authorGroupResponse
}

func toAuthorGroupResponse(g db.AuthorGroup) authorGroupResponse {
	if g.Members == nil {
		g.Members = []string{}
	}
	return authorGroupResponse{
		ID:        g.ID,
		Name:      g.Name,
		Members:   g.Members,
		CreatedAt: g.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: g.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Server) listAuthorGroups(ctx context.Context, _ *struct{}) (*listAuthorGroupsOutput, error) {
	groups, err := s.db.ListAuthorGroups(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("list author groups: " + err.Error())
	}
	resp := authorGroupsResponse{Groups: make([]authorGroupResponse, 0, len(groups))}
	for _, g := range groups {
		resp.Groups = append(resp.Groups, toAuthorGroupResponse(g))
	}
	return &listAuthorGroupsOutput{Body: resp}, nil
}

func (s *Server) createAuthorGroup(ctx context.Context, input *createAuthorGroupInput) (*authorGroupOutput, error) {
	if len(input.Body.Members) > authorGroupMaxMembers {
		return nil, huma.Error413RequestEntityTooLarge("too many members (max 500)")
	}
	g, err := s.db.CreateAuthorGroup(ctx, input.Body.Name, input.Body.Members)
	if err != nil {
		if errors.Is(err, db.ErrAuthorGroupNameTaken) {
			return nil, huma.Error409Conflict("an author group named '" + input.Body.Name + "' already exists")
		}
		return nil, huma.Error400BadRequest(err.Error())
	}
	return &authorGroupOutput{Body: toAuthorGroupResponse(g)}, nil
}

func (s *Server) updateAuthorGroup(ctx context.Context, input *updateAuthorGroupInput) (*authorGroupOutput, error) {
	if len(input.Body.Members) > authorGroupMaxMembers {
		return nil, huma.Error413RequestEntityTooLarge("too many members (max 500)")
	}
	g, err := s.db.UpdateAuthorGroup(ctx, input.ID, input.Body.Name, input.Body.Members)
	if err != nil {
		if errors.Is(err, db.ErrAuthorGroupNameTaken) {
			return nil, huma.Error409Conflict("an author group named '" + input.Body.Name + "' already exists")
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, huma.Error404NotFound("author group not found")
		}
		return nil, huma.Error400BadRequest(err.Error())
	}
	return &authorGroupOutput{Body: toAuthorGroupResponse(g)}, nil
}

func (s *Server) deleteAuthorGroup(ctx context.Context, input *authorGroupPathInput) (*emptyOutput, error) {
	if err := s.db.DeleteAuthorGroup(ctx, input.ID); err != nil {
		return nil, huma.Error500InternalServerError("delete author group: " + err.Error())
	}
	return &emptyOutput{}, nil
}
