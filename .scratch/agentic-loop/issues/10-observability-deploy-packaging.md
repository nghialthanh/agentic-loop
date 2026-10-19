# 10: Observability and deploy packaging

**What to build:** Structured logs include `message_id`, `conversation_id`, and `trace_id`. Each attempt gets an OpenTelemetry trace. Metrics cover end-to-end latency, tool-call success rate, token usage, and hallucination count. Dockerfile, required environment variables, and notes for adding consumer replicas ship so the worker is runnable and operable.

**Blocked by:** 09 (Kafka consumer and publisher)

**Status:** resolved

- [x] Logs carry `message_id`, `conversation_id`, and `trace_id` per attempt
- [x] A new `trace_id` is issued per attempt
- [x] Metrics expose latency, tool success, tokens, and hallucinations
- [x] PII redaction applies to logs and traces
- [x] Dockerfile builds a runnable image
- [x] Environment variables and replica scaling notes are documented

## Answer

`internal/observability` provides structured JSON logs (`message_id`, `conversation_id`, `trace_id`) with PII redaction, one OpenTelemetry `attempt` span per processing attempt, and `/metrics` for latency, tool success/error, tokens, and hallucinations. Attempt wires optional `Telemetry`; `cmd/worker` registers health+metrics. `Dockerfile` and `docs/deploy.md` cover env vars and horizontal replicas in group `agentic-loop`.
