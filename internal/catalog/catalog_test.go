package catalog_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/agentic-loop/worker/internal/catalog"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Username("catalog").
		Password("catalog").
		Database("catalog").
		Port(5433))
	if err := pg.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start postgres: %v\n", err)
		os.Exit(1)
	}

	db, err := sql.Open("pgx", "postgres://catalog:catalog@localhost:5433/catalog?sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		_ = pg.Stop()
		os.Exit(1)
	}
	testDB = db

	code := m.Run()
	_ = db.Close()
	_ = pg.Stop()
	os.Exit(code)
}

func resetCatalog(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	if _, err := testDB.ExecContext(ctx, `DROP TABLE IF EXISTS tools; DROP TABLE IF EXISTS skills;`); err != nil {
		t.Fatalf("drop tables: %v", err)
	}
	if err := catalog.Migrate(ctx, testDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return testDB
}

func TestFixtureLoadMakesOrderSupportToolsRetrievable(t *testing.T) {
	db := resetCatalog(t)
	ctx := context.Background()
	repo := catalog.NewRepository(db)

	if err := catalog.SeedOrderSupport(ctx, db); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}

	tools, err := repo.ToolsForSkills(ctx, []string{"order_support"})
	if err != nil {
		t.Fatalf("ToolsForSkills: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(tools))
	}
	if tools[0].Name != "get_order_status" {
		t.Fatalf("first tool = %q, want get_order_status", tools[0].Name)
	}
	if tools[1].Name != "refund_order" {
		t.Fatalf("second tool = %q, want refund_order", tools[1].Name)
	}
	if len(tools[1].Requires) != 1 || tools[1].Requires[0] != "get_order_status" {
		t.Fatalf("refund_order.requires = %#v, want [get_order_status]", tools[1].Requires)
	}
}

func TestSearchSkillsReturnsMatchingEnabledSkills(t *testing.T) {
	db := resetCatalog(t)
	ctx := context.Background()
	repo := catalog.NewRepository(db)

	if err := catalog.SeedOrderSupport(ctx, db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	insertSkill(t, db, "billing_help", "Use when the customer asks about invoices or payment methods.", true)
	insertSkill(t, db, "disabled_orders", "Use when the customer asks about order shipping status.", false)

	skills, err := repo.SearchSkills(ctx, "refund order shipped")
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "order_support" {
		t.Fatalf("skill = %q, want order_support", skills[0].Name)
	}
}

func TestSearchSkillsCapsAtTwenty(t *testing.T) {
	db := resetCatalog(t)
	ctx := context.Background()
	repo := catalog.NewRepository(db)

	for i := 0; i < 25; i++ {
		insertSkill(t, db, fmt.Sprintf("shipping_skill_%02d", i), "Handles parcel shipping tracking questions.", true)
	}

	skills, err := repo.SearchSkills(ctx, "shipping tracking")
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(skills) != 20 {
		t.Fatalf("got %d skills, want 20", len(skills))
	}
}

func TestToolsForSkillsExcludesDisabledToolsAndOtherSkills(t *testing.T) {
	db := resetCatalog(t)
	ctx := context.Background()
	repo := catalog.NewRepository(db)

	if err := catalog.SeedOrderSupport(ctx, db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	billingID := insertSkill(t, db, "billing_help", "Invoices and payments.", true)
	insertTool(t, db, billingID, "get_invoice", true)
	if _, err := db.ExecContext(ctx, `UPDATE tools SET enabled = FALSE WHERE name = 'refund_order'`); err != nil {
		t.Fatalf("disable refund_order: %v", err)
	}

	tools, err := repo.ToolsForSkills(ctx, []string{"order_support"})
	if err != nil {
		t.Fatalf("ToolsForSkills: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "get_order_status" {
		t.Fatalf("tools = %#v, want only get_order_status", toolNames(tools))
	}
}

func TestToolsForSkillsExcludesDisabledSkills(t *testing.T) {
	db := resetCatalog(t)
	ctx := context.Background()
	repo := catalog.NewRepository(db)

	if err := catalog.SeedOrderSupport(ctx, db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE skills SET enabled = FALSE WHERE name = 'order_support'`); err != nil {
		t.Fatalf("disable skill: %v", err)
	}

	tools, err := repo.ToolsForSkills(ctx, []string{"order_support"})
	if err != nil {
		t.Fatalf("ToolsForSkills: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("tools = %#v, want none for disabled skill", toolNames(tools))
	}
}

func insertSkill(t *testing.T, db *sql.DB, name, description string, enabled bool) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO skills (name, description, enabled)
		VALUES ($1, $2, $3)
		RETURNING id
	`, name, description, enabled).Scan(&id)
	if err != nil {
		t.Fatalf("insert skill %q: %v", name, err)
	}
	return id
}

func insertTool(t *testing.T, db *sql.DB, skillID int64, name string, enabled bool) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO tools (
			skill_id, name, description, method, endpoint, auth, timeout_ms,
			enabled, parameters, response, requires
		) VALUES (
			$1, $2, 'test tool', 'GET', 'https://api.internal/x', 'bearer', 1000,
			$3, '{}'::jsonb, '{"keep":[]}'::jsonb, '{}'
		)
	`, skillID, name, enabled)
	if err != nil {
		t.Fatalf("insert tool %q: %v", name, err)
	}
}

func toolNames(tools []catalog.Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}
