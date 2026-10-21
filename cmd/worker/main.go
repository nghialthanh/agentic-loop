package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/agentic-loop/worker/internal/service/attempt"
	"github.com/agentic-loop/worker/internal/repository/catalog"
	"github.com/agentic-loop/worker/internal/adapter/health"
	kafkaworker "github.com/agentic-loop/worker/internal/adapter/kafka"
	"github.com/agentic-loop/worker/internal/adapter/model"
	"github.com/agentic-loop/worker/internal/adapter/observability"
	"github.com/agentic-loop/worker/internal/repository/store"
	"github.com/agentic-loop/worker/internal/adapter/tools"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	tel := observability.NewDefault()

	mux := http.NewServeMux()
	health.RegisterAll(mux, tel.Metrics)

	addr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if addr == "" {
		addr = ":8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("listening on %s", addr)
		server := &http.Server{Addr: addr, Handler: mux}
		go func() {
			<-ctx.Done()
			_ = server.Shutdown(context.Background())
		}()
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	brokersEnv := strings.TrimSpace(os.Getenv("KAFKA_BROKERS"))
	if brokersEnv == "" {
		log.Println("KAFKA_BROKERS unset; health/metrics server only")
		<-ctx.Done()
		return
	}
	brokers := strings.Split(brokersEnv, ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required when KAFKA_BROKERS is set")
	}
	modelBase := strings.TrimSpace(os.Getenv("MODEL_BASE_URL"))
	if modelBase == "" {
		log.Fatal("MODEL_BASE_URL is required when KAFKA_BROKERS is set")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}
	if err := catalog.Migrate(ctx, db); err != nil {
		log.Fatalf("catalog migrate: %v", err)
	}
	if err := store.Migrate(ctx, db); err != nil {
		log.Fatalf("store migrate: %v", err)
	}

	modelName := strings.TrimSpace(os.Getenv("MODEL"))
	a := attempt.New(attempt.Deps{
		Catalog: catalog.NewRepository(db),
		Model: &model.OpenAI{
			BaseURL: modelBase,
			APIKey:  strings.TrimSpace(os.Getenv("MODEL_API_KEY")),
			Model:   modelName,
		},
		Tools: &tools.HTTPExecutor{
			BearerToken: strings.TrimSpace(os.Getenv("TOOL_BEARER_TOKEN")),
		},
		Store:     store.NewPostgres(db),
		Clock:     systemClock{},
		ModelName: modelName,
		Telemetry: tel,
	})
	bus := kafkaworker.NewKafkaBus(brokers, kafkaworker.GroupID)
	defer bus.Close()

	log.Printf("consuming %s as group %s", kafkaworker.TopicInput, kafkaworker.GroupID)
	consumer := &kafkaworker.Consumer{
		Worker: kafkaworker.NewWorker(a, bus),
		Bus:    bus,
	}
	if err := consumer.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
