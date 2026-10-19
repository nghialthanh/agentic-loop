# Deploying the agentic-loop worker

## Dockerfile

Build from the repo root:

```bash
docker build -t agentic-loop-worker .
docker run --rm -p 8080:8080 \
  -e KAFKA_BROKERS=kafka:9092 \
  -e TOOL_BEARER_TOKEN=secret \
  -e MODEL=gpt-4o-mini \
  agentic-loop-worker
```

The process exposes `GET /health` and `GET /metrics` on `HTTP_ADDR` (default `:8080`).

## Environment variables

| Variable | Required | Description |
| --- | --- | --- |
| `KAFKA_BROKERS` | Yes (for consuming) | Comma-separated Kafka broker addresses. If unset, only the health/metrics HTTP server runs. |
| `HTTP_ADDR` | No | Listen address for health and metrics (default `:8080`). |
| `TOOL_BEARER_TOKEN` | Yes (when calling bearer tools) | Shared bearer credential for catalog tools with `auth: bearer`. Never stored in the skill catalog. |
| `MODEL` | Recommended | Model id for skill selection and tool-calling (temperature `0`). |
| `DATABASE_URL` | Yes (production) | Postgres URL for the skill catalog, dedupe claims, and per-customer rate limits. Shared across replicas. |

Kafka topic and group names are fixed in code: input `chat.customer.messages`, output `chat.agent.replies`, DLQ `chat.customer.messages.dlq`, consumer group `agentic-loop`.

## Adding consumer replicas

1. Deploy another identical pod/process with the same `KAFKA_BROKERS`, `DATABASE_URL`, and secrets.
2. Keep the consumer group id `agentic-loop` (do not invent a per-pod group).
3. Do not pin conversations to a pod. Partition assignment and the shared Postgres claim/dedupe store coordinate work.
4. Scale out by increasing replica count; scale in the same way. Each replica runs its own health/metrics server.
5. Ensure the input topic has enough partitions for the desired parallelism (one active consumer per partition in the group).

Structured logs include `message_id`, `conversation_id`, and `trace_id` per attempt. Each attempt starts a new OpenTelemetry span named `attempt`. Metrics cover end-to-end latency, tool-call success/error counts, token usage, and hallucination count.
