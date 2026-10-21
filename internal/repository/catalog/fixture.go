package catalog

import (
	"context"
	"database/sql"
)

// SeedOrderSupport inserts the foundation order_support skill and its tools.
// get_order_status is first; refund_order requires get_order_status.
func SeedOrderSupport(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var skillID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO skills (name, description, enabled)
		VALUES ($1, $2, TRUE)
		ON CONFLICT (name) DO UPDATE SET
			description = EXCLUDED.description,
			enabled = EXCLUDED.enabled,
			updated_at = now()
		RETURNING id
	`,
		"order_support",
		"Use when the customer asks where an order is, whether it shipped, or for a refund on an order.",
	).Scan(&skillID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO tools (
			skill_id, name, description, method, endpoint, auth, timeout_ms,
			enabled, parameters, response, requires
		) VALUES (
			$1, 'get_order_status', 'Get the status of one order.',
			'GET', 'https://api.internal/orders/{order_id}', 'bearer', 3000, TRUE,
			$2::jsonb, $3::jsonb, '{}'
		)
		ON CONFLICT (name) DO UPDATE SET
			skill_id = EXCLUDED.skill_id,
			description = EXCLUDED.description,
			method = EXCLUDED.method,
			endpoint = EXCLUDED.endpoint,
			auth = EXCLUDED.auth,
			timeout_ms = EXCLUDED.timeout_ms,
			enabled = EXCLUDED.enabled,
			parameters = EXCLUDED.parameters,
			response = EXCLUDED.response,
			requires = EXCLUDED.requires,
			updated_at = now()
	`,
		skillID,
		`{"type":"object","additionalProperties":false,"required":["order_id"],"properties":{"order_id":{"type":"string"}}}`,
		`{"keep":["order_id","status","carrier"]}`,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO tools (
			skill_id, name, description, method, endpoint, auth, timeout_ms,
			enabled, parameters, response, requires
		) VALUES (
			$1, 'refund_order', 'Refund one order after its status is known.',
			'POST', 'https://api.internal/orders/{order_id}/refunds', 'bearer', 3000, TRUE,
			$2::jsonb, $3::jsonb, ARRAY['get_order_status']
		)
		ON CONFLICT (name) DO UPDATE SET
			skill_id = EXCLUDED.skill_id,
			description = EXCLUDED.description,
			method = EXCLUDED.method,
			endpoint = EXCLUDED.endpoint,
			auth = EXCLUDED.auth,
			timeout_ms = EXCLUDED.timeout_ms,
			enabled = EXCLUDED.enabled,
			parameters = EXCLUDED.parameters,
			response = EXCLUDED.response,
			requires = EXCLUDED.requires,
			updated_at = now()
	`,
		skillID,
		`{"type":"object","additionalProperties":false,"required":["order_id","reason","customer_id"],"properties":{"order_id":{"type":"string"},"reason":{"type":"string"},"customer_id":{"type":"string"}}}`,
		`{"keep":["refund_id","status"]}`,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}
