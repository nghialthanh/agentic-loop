# Agentic Loop Worker

Kafka in, Kafka out customer-support worker. The legacy chat owns the conversation UI; this service consumes one customer message, runs one **attempt** (skill selection + bounded ReAct tool loop), and publishes one reply (or a DLQ record for malformed input).

Go module: `github.com/agentic-loop/worker`

## What it does

1. Consume `chat.customer.messages`
2. Validate input; malformed payloads go to `chat.customer.messages.dlq` only
3. Guard, rate-limit, and claim the message (Postgres-backed dedupe)
4. Retrieve skill candidates, select a skill, run tools via HTTP, compose a reply grounded in tool results
5. Publish to `chat.agent.replies` and commit the Kafka offset

Replicas share consumer group `agentic-loop` and the same Postgres store for claims and rate limits.

## Quick start

### Prerequisites

- Go 1.27+
- Docker (for image builds / integration tests)
- Kafka and Postgres when running as a consumer

### Build & test

```bash
go build -o bin/worker ./cmd/worker
go test ./...
```

### Run locally

```bash
export KAFKA_BROKERS=localhost:9092
export DATABASE_URL=postgres://worker:worker@localhost:5432/worker?sslmode=disable
export MODEL_BASE_URL=https://api.openai.com/v1
export MODEL_API_KEY=sk-...
export TOOL_BEARER_TOKEN=secret
export MODEL=gpt-4o-mini

go run ./cmd/worker
```

Health and metrics listen on `HTTP_ADDR` (default `:8080`):

- `GET /health`
- `GET /metrics`

If `KAFKA_BROKERS` is unset, only the HTTP server starts (no Kafka / DB / model wiring).

### Docker

```bash
docker build -t agentic-loop-worker .
docker run --rm -p 8080:8080 \
  -e KAFKA_BROKERS=kafka:9092 \
  -e DATABASE_URL=postgres://worker:worker@db:5432/worker?sslmode=disable \
  -e MODEL_BASE_URL=https://api.openai.com/v1 \
  -e MODEL_API_KEY=sk-... \
  -e TOOL_BEARER_TOKEN=secret \
  -e MODEL=gpt-4o-mini \
  agentic-loop-worker
```

Full env table and replica guidance: [docs/deploy.md](docs/deploy.md).

## Architecture

Three layers; dependencies point inward (`adapter → service → repository`):

| Layer | Path | Role |
| --- | --- | --- |
| Repository | `internal/repository/` | Postgres DAOs (skill catalog, claims, rate limits) |
| Service | `internal/service/attempt/` | Attempt logic: guard, claim, selection, ReAct, reply |
| Adapter | `internal/adapter/` | Kafka, model HTTP, tool HTTP, health, observability |

Shared Kafka DTOs live in `internal/contract`. Composition root: `cmd/worker`.

```
adapter/kafka (chat.customer.messages)
  → service/attempt
      → repository/catalog , repository/store
      → adapter/model , adapter/tools
  → adapter/kafka (chat.agent.replies | chat.customer.messages.dlq)
```

## Kafka topics

| Topic | Purpose |
| --- | --- |
| `chat.customer.messages` | Input (key = `message_id`) |
| `chat.agent.replies` | Successful / handled outputs |
| `chat.customer.messages.dlq` | Malformed input only |

Message shapes and delivery rules: [foudation_prompt.md](foudation_prompt.md) §2.

## Docs

| Doc | Contents |
| --- | --- |
| [docs/structure.md](docs/structure.md) | Package layout and runtime flow |
| [docs/deploy.md](docs/deploy.md) | Env vars, Docker, scaling replicas |
| [docs/error-handling.md](docs/error-handling.md) | Error codes and handling matrix |
| [GLOSSARY.md](GLOSSARY.md) | Domain terms (worker, skill, tool, attempt, …) |
| [docs/adr/](docs/adr/) | Architecture decisions |
| [AGENTS.md](AGENTS.md) | Notes for coding agents working in this repo |
| [.scratch/agentic-loop/](.scratch/agentic-loop/) | Spec and issue tracker |

## License

Proprietary — internal use unless otherwise stated.
