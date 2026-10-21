# Repository structure

Go module: `github.com/agentic-loop/worker`. One binary: the Kafka worker that runs one **attempt** per customer message and publishes one reply (or DLQ).

Code follows three layers. Dependencies point inward: **adapter → service → repository**. Shared message DTOs live in `internal/contract` (used by all layers).

## Layers

| Layer | Path | Role |
| --- | --- | --- |
| **Repository (DAO)** | `internal/repository/` | Direct queries/commands to Postgres (and in-memory store fakes for tests). |
| **Service (logic)** | `internal/service/` | Business logic for one attempt: guard, claim, skill selection, ReAct, reply. |
| **Adapter** | `internal/adapter/` | External I/O: Kafka, model HTTP API, tool HTTP, health HTTP, observability. |

`cmd/worker` wires the three layers from environment config.

## Tree

```
.
├── cmd/worker/                 # composition root
├── internal/
│   ├── contract/               # shared Kafka input/output DTOs + validation
│   ├── repository/             # layer 1 — DAO
│   │   ├── catalog/            # skills/tools tables, FTS top-20, fixture
│   │   └── store/              # claims, saved replies, rate limits
│   ├── service/                # layer 2 — logic
│   │   └── attempt/            # primary seam: message → output or DLQ
│   └── adapter/                # layer 3 — api-call / kafka / HTTP
│       ├── kafka/              # consume, publish replies/DLQ, commit offset
│       ├── model/              # OpenAI-compatible model client
│       ├── tools/              # outbound tool HTTP executor
│       ├── health/             # GET /health, /metrics mount
│       └── observability/      # structured logs, OTel, metrics
├── docs/
│   ├── structure.md
│   ├── deploy.md
│   ├── error-handling.md
│   ├── adr/
│   └── agents/
├── Dockerfile
├── GLOSSARY.md
├── AGENTS.md
├── foudation_prompt.md
└── .scratch/agentic-loop/
    ├── spec.md
    └── issues/
```

## Package roles

| Package | Layer | Responsibility |
| --- | --- | --- |
| `internal/repository/catalog` | repository | Skill/tool SQL, FTS candidate search, `order_support` fixture. |
| `internal/repository/store` | repository | Claim / SaveOutput / rate-limit persistence (memory + Postgres). |
| `internal/service/attempt` | service | Guard, rate limit, claim, selection, ReAct loop, composer. Defines ports (Catalog, Model, Tools, Store, Clock). Tests drive behaviour here with fakes. |
| `internal/adapter/kafka` | adapter | Topics/group, bus, consume → service → publish → commit. |
| `internal/adapter/model` | adapter | Model HTTP client implementing the service Model port. |
| `internal/adapter/tools` | adapter | Tool HTTP client implementing the service Tools port. |
| `internal/adapter/health` | adapter | Operability HTTP beside the consumer. |
| `internal/adapter/observability` | adapter | Logs (`message_id`, `conversation_id`, `trace_id`), traces, metrics. |
| `internal/contract` | shared | Input/output message shapes and schema validation. |
| `cmd/worker` | root | Wires repository + service + adapters from env. |

## Runtime flow

```
adapter/kafka (chat.customer.messages)
  → service/attempt.Run / RunBytes
      → repository/catalog , repository/store
      → adapter/model , adapter/tools   (via ports)
  → adapter/kafka publish chat.agent.replies | chat.customer.messages.dlq
  → commit offset (after store→publish for accepted messages)
```

Malformed input never claims or publishes a normal output; it goes to the DLQ only.

## Where to look

| Question | Place |
| --- | --- |
| Field names / Kafka payload | `internal/contract`, `foudation_prompt.md` §2 |
| Attempt behaviour / errors | `internal/service/attempt`, `docs/error-handling.md` |
| Skill catalog SQL | `internal/repository/catalog`, ADRs under `docs/adr/` |
| Dedupe / rate limit SQL | `internal/repository/store`, ADR 0003 / 0005 |
| Topics & consumer group | `internal/adapter/kafka/names.go`, `docs/deploy.md` |
| Domain words | `GLOSSARY.md` |
| Spec & tickets | `.scratch/agentic-loop/` |
