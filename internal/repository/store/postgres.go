package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/agentic-loop/worker/internal/contract"
)

// Postgres is the shared-store adapter backed by the catalog database.
type Postgres struct {
	db       *sql.DB
	ClaimTTL time.Duration
	Limit    int
	Window   time.Duration
}

// NewPostgres builds a store over db with default TTL and rate limit.
func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{
		db:       db,
		ClaimTTL: DefaultClaimTTL,
		Limit:    DefaultRateLimit,
		Window:   DefaultRateWindow,
	}
}

func (p *Postgres) claimTTL() time.Duration {
	if p.ClaimTTL <= 0 {
		return DefaultClaimTTL
	}
	return p.ClaimTTL
}

func (p *Postgres) limit() int {
	if p.Limit <= 0 {
		return DefaultRateLimit
	}
	return p.Limit
}

func (p *Postgres) window() time.Duration {
	if p.Window <= 0 {
		return DefaultRateWindow
	}
	return p.Window
}

// Claim acquires or observes the claim for messageID at now.
func (p *Postgres) Claim(ctx context.Context, messageID string, now time.Time) (ClaimResult, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return ClaimResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var expiresAt time.Time
	var outputJSON []byte
	err = tx.QueryRowContext(ctx, `
		SELECT expires_at, output FROM message_claims WHERE message_id = $1 FOR UPDATE
	`, messageID).Scan(&expiresAt, &outputJSON)
	if err == sql.ErrNoRows {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO message_claims (message_id, expires_at, output, updated_at)
			VALUES ($1, $2, NULL, $3)
		`, messageID, now.Add(p.claimTTL()), now)
		if err != nil {
			return ClaimResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return ClaimResult{}, err
		}
		return ClaimResult{Outcome: ClaimWon}, nil
	}
	if err != nil {
		return ClaimResult{}, err
	}

	if len(outputJSON) > 0 && string(outputJSON) != "null" {
		var out contract.OutputMessage
		if err := json.Unmarshal(outputJSON, &out); err != nil {
			return ClaimResult{}, fmt.Errorf("decode stored output: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return ClaimResult{}, err
		}
		return ClaimResult{Outcome: ClaimStored, Stored: &out}, nil
	}

	if now.Before(expiresAt) {
		if err := tx.Commit(); err != nil {
			return ClaimResult{}, err
		}
		return ClaimResult{Outcome: ClaimHeld}, nil
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE message_claims
		SET expires_at = $2, output = NULL, updated_at = $3
		WHERE message_id = $1
	`, messageID, now.Add(p.claimTTL()), now)
	if err != nil {
		return ClaimResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ClaimResult{}, err
	}
	return ClaimResult{Outcome: ClaimWon}, nil
}

// SaveOutput stores the output JSON for a claimed message_id.
func (p *Postgres) SaveOutput(ctx context.Context, out contract.OutputMessage) error {
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	res, err := p.db.ExecContext(ctx, `
		UPDATE message_claims
		SET output = $2, updated_at = now()
		WHERE message_id = $1
	`, out.MessageID, raw)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("save output: no claim for %s", out.MessageID)
	}
	return nil
}

// ReserveRateLimit counts messageID for customerID if under the rolling limit.
func (p *Postgres) ReserveRateLimit(ctx context.Context, customerID, messageID string, now time.Time) (bool, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var existing string
	err = tx.QueryRowContext(ctx, `
		SELECT message_id FROM rate_limit_events WHERE message_id = $1
	`, messageID).Scan(&existing)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}
	if err != sql.ErrNoRows {
		return false, err
	}

	windowStart := now.Add(-p.window())
	var count int
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM rate_limit_events
		WHERE customer_id = $1 AND counted_at >= $2
	`, customerID, windowStart).Scan(&count)
	if err != nil {
		return false, err
	}
	if count >= p.limit() {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO rate_limit_events (message_id, customer_id, counted_at)
		VALUES ($1, $2, $3)
	`, messageID, customerID, now)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
