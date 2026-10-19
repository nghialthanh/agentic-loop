package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/agentic-loop/worker/internal/attempt"
	"github.com/agentic-loop/worker/internal/catalog"
	"github.com/agentic-loop/worker/internal/health"
	kafkaworker "github.com/agentic-loop/worker/internal/kafka"
	"github.com/agentic-loop/worker/internal/observability"
	"github.com/agentic-loop/worker/internal/store"
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

	// Catalog / model / tools adapters are wired in later tickets; memory store
	// is enough for the Kafka consumer loop to run against a broker.
	a := attempt.New(attempt.Deps{
		Catalog:   noopCatalog{},
		Model:     noopModel{},
		Tools:     noopTools{},
		Store:     store.NewMemory(),
		Clock:     systemClock{},
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

type noopCatalog struct{}

func (noopCatalog) SearchSkills(context.Context, string) ([]catalog.Skill, error) {
	return nil, nil
}
func (noopCatalog) ToolsForSkills(context.Context, []string) ([]catalog.Tool, error) {
	return nil, nil
}

type noopModel struct{}

func (noopModel) SelectSkills(context.Context, attempt.SkillSelectionCall) (string, error) {
	return `{"intent":"general","skill_names":[],"reason":"none"}`, nil
}
func (noopModel) CallWithTools(context.Context, attempt.ToolCallingCall) (attempt.ToolCallingResult, error) {
	return attempt.ToolCallingResult{Reply: attempt.GeneralReply}, nil
}

type noopTools struct{}

func (noopTools) Execute(context.Context, attempt.ToolHTTPRequest) attempt.ToolHTTPResponse {
	return attempt.ToolHTTPResponse{}
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
