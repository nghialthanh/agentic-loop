# 10: Observability and deploy packaging

**What to build:** Structured logs include `message_id`, `conversation_id`, and `trace_id`. Each attempt gets an OpenTelemetry trace. Metrics cover end-to-end latency, tool-call success rate, token usage, and hallucination count. Dockerfile, required environment variables, and notes for adding consumer replicas ship so the worker is runnable and operable.

**Blocked by:** 09 (Kafka consumer and publisher)

**Status:** ready-for-agent

- [ ] Logs carry `message_id`, `conversation_id`, and `trace_id` per attempt
- [ ] A new `trace_id` is issued per attempt
- [ ] Metrics expose latency, tool success, tokens, and hallucinations
- [ ] PII redaction applies to logs and traces
- [ ] Dockerfile builds a runnable image
- [ ] Environment variables and replica scaling notes are documented
