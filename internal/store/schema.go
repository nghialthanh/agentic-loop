package store

import (
	"context"
	"database/sql"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS message_claims (
    message_id TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL,
    output JSONB NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rate_limit_events (
    message_id TEXT PRIMARY KEY,
    customer_id TEXT NOT NULL,
    counted_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS rate_limit_events_customer_counted_idx
    ON rate_limit_events (customer_id, counted_at);
`

// Migrate creates claim and rate-limit tables in the shared catalog database.
func Migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, schemaSQL)
	return err
}
