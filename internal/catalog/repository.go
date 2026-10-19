package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/lib/pq"
)

// Skill is a named group of tools for one kind of customer request.
type Skill struct {
	ID          int64
	Name        string
	Description string
	Enabled     bool
	UpdatedAt   time.Time
}

// Tool is a single operation the worker can call for one skill.
type Tool struct {
	ID          int64
	SkillID     int64
	Name        string
	Description string
	Method      string
	Endpoint    string
	Auth        string
	TimeoutMS   int
	Enabled     bool
	Parameters  json.RawMessage
	Response    json.RawMessage
	Requires    []string
	UpdatedAt   time.Time
}

// Repository reads skills and tools from Postgres.
type Repository struct {
	db *sql.DB
}

// NewRepository builds a catalog repository over db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

const maxSkillCandidates = 20

// SearchSkills returns up to 20 enabled skills matching query over name and description.
func (r *Repository) SearchSkills(ctx context.Context, query string) ([]Skill, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, enabled, updated_at
		FROM skills
		WHERE enabled = TRUE
		  AND to_tsvector('english', coalesce(name, '') || ' ' || coalesce(description, ''))
		      @@ plainto_tsquery('english', $1)
		ORDER BY ts_rank(
			to_tsvector('english', coalesce(name, '') || ' ' || coalesce(description, '')),
			plainto_tsquery('english', $1)
		) DESC, name ASC
		LIMIT $2
	`, query, maxSkillCandidates)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Skill
	for rows.Next() {
		var s Skill
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Enabled, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ToolsForSkills loads enabled tools for the named skills, ordered by skill name then tool id.
func (r *Repository) ToolsForSkills(ctx context.Context, skillNames []string) ([]Tool, error) {
	if len(skillNames) == 0 {
		return nil, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.skill_id, t.name, t.description, t.method, t.endpoint, t.auth,
		       t.timeout_ms, t.enabled, t.parameters, t.response, t.requires, t.updated_at
		FROM tools t
		INNER JOIN skills s ON s.id = t.skill_id
		WHERE s.name = ANY($1)
		  AND s.enabled = TRUE
		  AND t.enabled = TRUE
		ORDER BY s.name ASC, t.id ASC
	`, pq.Array(skillNames))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Tool
	for rows.Next() {
		var tool Tool
		var requires pq.StringArray
		if err := rows.Scan(
			&tool.ID, &tool.SkillID, &tool.Name, &tool.Description, &tool.Method, &tool.Endpoint, &tool.Auth,
			&tool.TimeoutMS, &tool.Enabled, &tool.Parameters, &tool.Response, &requires, &tool.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tool.Requires = []string(requires)
		if tool.Requires == nil {
			tool.Requires = []string{}
		}
		out = append(out, tool)
	}
	return out, rows.Err()
}
