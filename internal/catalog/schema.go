package catalog

import (
	"context"
	"database/sql"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS skills (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tools (
    id BIGSERIAL PRIMARY KEY,
    skill_id BIGINT NOT NULL REFERENCES skills(id),
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    method TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    auth TEXT NOT NULL,
    timeout_ms INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    parameters JSONB NOT NULL,
    response JSONB NOT NULL,
    requires TEXT[] NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS skills_fts_idx ON skills
    USING GIN (to_tsvector('english', coalesce(name, '') || ' ' || coalesce(description, '')));
`

// Migrate creates the skills and tools catalog tables.
func Migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, schemaSQL)
	return err
}
