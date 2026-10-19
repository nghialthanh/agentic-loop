package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/agentic-loop/worker/internal/contract"
	"github.com/agentic-loop/worker/internal/store"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	runtimePath, err := os.MkdirTemp("", "store-pg-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "temp dir: %v\n", err)
		os.Exit(1)
	}
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Username("store").
		Password("store").
		Database("store").
		Port(5434).
		RuntimePath(runtimePath))
	if err := pg.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start postgres: %v\n", err)
		_ = os.RemoveAll(runtimePath)
		os.Exit(1)
	}

	db, err := sql.Open("pgx", "postgres://store:store@localhost:5434/store?sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		_ = pg.Stop()
		os.Exit(1)
	}
	testDB = db

	code := m.Run()
	_ = db.Close()
	_ = pg.Stop()
	_ = os.RemoveAll(runtimePath)
	os.Exit(code)
}

func resetStore(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	if _, err := testDB.ExecContext(ctx, `
		DROP TABLE IF EXISTS rate_limit_events;
		DROP TABLE IF EXISTS message_claims;
	`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := store.Migrate(ctx, testDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return testDB
}

func TestPostgres_claimStoreAndRedelivery(t *testing.T) {
	db := resetStore(t)
	pg := store.NewPostgres(db)
	pg.ClaimTTL = 30 * time.Second
	ctx := context.Background()
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

	won, err := pg.Claim(ctx, "msg_1", now)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if won.Outcome != store.ClaimWon {
		t.Fatalf("outcome = %v, want Won", won.Outcome)
	}

	held, err := pg.Claim(ctx, "msg_1", now.Add(time.Second))
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if held.Outcome != store.ClaimHeld {
		t.Fatalf("outcome = %v, want Held", held.Outcome)
	}

	out := contract.OutputMessage{
		MessageID:      "msg_1",
		ConversationID: "conv",
		Reply:          "stored",
		Skills:         []string{},
		UsedTools:      []contract.UsedTool{},
		TraceID:        "trace-a",
	}
	if err := pg.SaveOutput(ctx, out); err != nil {
		t.Fatalf("save: %v", err)
	}

	stored, err := pg.Claim(ctx, "msg_1", now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("redelivery claim: %v", err)
	}
	if stored.Outcome != store.ClaimStored || stored.Stored == nil || stored.Stored.Reply != "stored" {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestPostgres_expiredClaimCanBeTakenOver(t *testing.T) {
	db := resetStore(t)
	pg := store.NewPostgres(db)
	pg.ClaimTTL = 10 * time.Second
	ctx := context.Background()
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

	if _, err := pg.Claim(ctx, "msg_take", now); err != nil {
		t.Fatalf("claim: %v", err)
	}
	takeover, err := pg.Claim(ctx, "msg_take", now.Add(11*time.Second))
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if takeover.Outcome != store.ClaimWon {
		t.Fatalf("outcome = %v, want Won", takeover.Outcome)
	}
}

func TestPostgres_rateLimitRollingWindow(t *testing.T) {
	db := resetStore(t)
	pg := store.NewPostgres(db)
	pg.Limit = 2
	pg.Window = time.Minute
	ctx := context.Background()
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

	ok, err := pg.ReserveRateLimit(ctx, "cus_1", "m1", now)
	if err != nil || !ok {
		t.Fatalf("m1: ok=%v err=%v", ok, err)
	}
	ok, err = pg.ReserveRateLimit(ctx, "cus_1", "m2", now.Add(time.Second))
	if err != nil || !ok {
		t.Fatalf("m2: ok=%v err=%v", ok, err)
	}
	ok, err = pg.ReserveRateLimit(ctx, "cus_1", "m3", now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("m3: %v", err)
	}
	if ok {
		t.Fatal("m3 should be rate limited")
	}
	// Same message_id does not double-count when already recorded.
	ok, err = pg.ReserveRateLimit(ctx, "cus_1", "m1", now.Add(3*time.Second))
	if err != nil || !ok {
		t.Fatalf("m1 again: ok=%v err=%v", ok, err)
	}
}
