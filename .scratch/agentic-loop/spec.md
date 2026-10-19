Status: ready-for-agent

# Agentic loop worker

## Problem Statement

The legacy chat already owns the conversation UI and customer threads. It needs a separate worker that can take one customer message, look up account facts through declared tools, and publish one reply, without writing to legacy databases or inventing order, refund, balance, or profile facts.

## Solution

A Go microservice consumes customer messages from Kafka, runs a bounded ReAct attempt (guard, skill candidates, skill selection, tools, reply), and publishes one output record per valid message. Skills and tools live in Postgres. Dedupe and rate limits share that database so every replica sees the same claim. Account facts in the reply come only from successful tool results in that attempt.

## User Stories

1. As the legacy chat, I want to publish a customer message onto a Kafka topic, so that the worker can answer without owning the UI.
2. As the legacy chat, I want one output record per accepted message, so that every customer turn gets a reply even when tools or the model fail.
3. As the legacy chat, I want malformed messages on a dead-letter topic instead of a normal reply, so that bad payloads do not look like answered turns.
4. As the legacy chat, I want `message_id` and `conversation_id` copied onto the reply, so that I can join request and response.
5. As the legacy chat, I want `used_tools` without raw API bodies, so that I can audit what ran without leaking internal payloads.
6. As the legacy chat, I want `trace_id` on every output, so that I can look up the attempt in traces.
7. As the legacy chat, I want `error` null or a known code, so that I can show or escalate failures consistently.
8. As a customer, I want a reply grounded in tool results when I ask about my order, so that I am not told invented statuses.
9. As a customer, I want a general reply that invents no account facts when no skill matches, so that a miss does not fabricate data.
10. As a customer, I want an explicit refund request to be able to refund after a successful status read, so that I do not need a second confirmation turn.
11. As a customer, I want a complaint or status-only question not to refund, so that money does not move on implied intent.
12. As a customer, I want a failed tool to produce a reply that says what could not be completed, so that I am not given guessed facts.
13. As a customer, I want a rate-limited reply when I send too many messages, so that the worker stays fair across customers.
14. As a customer, I want a refusal when the input is blocked by the guard, so that jailbreaks and injections do not get a helpful answer.
15. As a worker operator, I want horizontal replicas in one consumer group, so that I can scale without pinning conversations to a pod.
16. As a worker operator, I want dedupe on `message_id` in a shared store, so that a redelivery does not call write tools twice or publish a second reply.
17. As a worker operator, I want a claim before any tool call, so that two replicas cannot process the same message at once.
18. As a worker operator, I want an expired claim to be takeable, so that a crashed attempt can finish.
19. As a worker operator, I want skills searched by full-text to a top-20 candidate list, so that the model never sees the whole catalog.
20. As a worker operator, I want tools loaded only after skills are chosen, so that workers do not hold the full catalog in the prompt.
21. As a worker operator, I want disabled tools left out of calling, so that I can turn a tool off without deleting it.
22. As a worker operator, I want `requires` on a write tool, so that the executor refuses a write until the required read succeeded in this attempt.
23. As a worker operator, I want the executor to ignore order status meaning, so that eligibility stays with the order API.
24. As a worker operator, I want `customer_id` bound from the message, so that the model cannot act on another customer.
25. As a worker operator, I want secrets in the environment, not the catalog, so that credentials never live beside skill rows.
26. As a worker operator, I want one bearer token for tools that declare `bearer` auth, so that the first design stays simple.
27. As a worker operator, I want structured logs with `message_id`, `conversation_id`, and `trace_id`, so that one attempt is searchable.
28. As a worker operator, I want metrics for latency, tool success, tokens, and hallucinations, so that groundedness is observable.
29. As a worker operator, I want a new `trace_id` per attempt, so that takeovers and retries are distinct in traces.
30. As a catalog editor, I want to add skills and tools in Postgres, so that new request types do not require a code change for the catalog shape.
31. As a catalog editor, I want a skill description treated as a label, not an instruction, so that catalog text cannot redirect the model.
32. As a model caller, I want two calls with different jobs, so that skill selection never sees tool endpoints or parameters.
33. As a model caller, I want invalid selection JSON treated as no skill match, so that a bad decode still yields a general reply.
34. As a model caller, I want history trimmed to the newest turns that fit the budget, so that long threads still fit one attempt.
35. As a tester, I want to drive one attempt through a single seam, so that behaviour is asserted on the output and on which tools ran.
36. As a deployer, I want a health and metrics HTTP server beside the consumer, so that the process is operable.
37. As a deployer, I want Kafka topic names fixed for input, output, and DLQ, so that the legacy chat and the worker agree on the contract.
38. As a support lead, I want `confidence` to reflect groundedness, not a calibrated probability, so that low scores flag thin replies.
39. As a support lead, I want `error.code = tool_failed` when a requested account fact is missing after a tool failure inside the budget, so that a polite failure is still marked incomplete.
40. As a support lead, I want a hallucination flag when the reply asserts a kept field value no successful tool result contained, so that invented ids are countable.

## Implementation Decisions

### Scope and vocabulary

- Build the worker described in `foudation_prompt.md`, using terms from `GLOSSARY.md` (worker, model, customer, agent, skill, intent, tool, read tool, write tool, message, attempt, reply, account fact).
- Honour ADRs 0001–0009.

### External contract

- Input topic: `chat.customer.messages`. Output topic: `chat.agent.replies`. Dead-letter topic: `chat.customer.messages.dlq`. Consumer group: `agentic-loop`. Record key: `message_id`.
- Input and output payloads match the foundation brief schemas. Validate input before processing and output before publishing. Kafka payloads are Go structs. Tool parameters stay JSON Schema from the catalog.
- Malformed input (schema failure, empty `content` after trim, unknown history role) goes to the DLQ with the original payload and validation reason. No normal output record. No dedupe row.
- Valid input always produces one stored output record, including guard, rate limit, tool, model, and timeout failures.

### Attempt seam

- The primary module is the attempt: one inbound message in; one output record or one dead-letter record out.
- Catalog, model, tool HTTP APIs, shared store (dedupe, claims, rate limits), and clock sit behind that seam as adapters.
- Tests assert on the output or DLQ and on which tool HTTP requests ran. They do not assert on private orchestration steps.

### Pipeline

1. Input guard: schema, sanitization, per-customer rate limit, prompt-injection / jailbreak / moderation block.
2. Candidate retrieval: Postgres full-text over enabled skills (`name` + `description`), top 20.
3. Skill selection: model call with the skill-selection prompt; decode JSON; drop names not in the candidate list; on decode failure treat as no skill match; do not retry with a larger catalog.
4. Tool load: enabled tools for chosen skills only; build OpenAI function-calling schema from those rows. A message keeps the rows it loaded.
5. Bounded ReAct loop: planner chooses next tool and arguments; executor validates arguments, enforces `requires`, binds `customer_id` from the message when the tool parameters include it, calls HTTP, normalizes to `response.keep`. At most four tool executions per attempt. Message timeout budget applies; tool timeouts and retries deduct from it.
6. Response composer: write `reply` from tool results and the model. No skill match → general answer, no invented account facts, empty `skills` and `used_tools`, low `confidence`, `error` null. Tool failure → customer-safe wording, no guessed facts; `error.code = tool_failed` when a requested account fact is missing (ADR 0006).
7. Output guard: schema check; PII redaction on logs and traces (customer reply may include tool-returned data for this customer).
8. Publisher: publish the stored output record.

### Write tools and account facts

- Write tools run only when the latest message explicitly asks for that change (ADR 0001). Enforcement is in the tool-calling prompt (ADR 0008). The executor does not classify the sentence.
- Account facts in the reply come only from successful tool results in this attempt (ADR 0002).
- Tool rows may declare `requires`: names of tools that must have returned ok earlier in this attempt. Executor refuses the HTTP call otherwise and records `used_tools[].status = error` (ADR 0004).
- Successful required read is enough; executor does not interpret status values (ADR 0007).
- When parameters include `customer_id`, omit it from the model schema and insert the message's `customer_id` before validation (ADR 0009).

### Idempotency and shared store

- Claim `message_id` before any tool call. Winner runs the attempt, stores the output, then publishes the stored record (ADR 0003).
- Duplicate delivery publishes the stored record and does not call tools.
- Other workers back off while the claim is live; after claim expiry they may take over.
- Dedupe, claims, and per-customer rate limits live in the same Postgres as the skill catalog (ADR 0005). No Redis in the first design.
- Rate limit is per `customer_id`, shared across replicas. Over limit → short reply, `error.code = rate_limited`, no model or tools. Only the claim winner counts the message. Redelivery publishes the stored reply and does not count again.
- Default rate limit: 10 messages per rolling minute (configurable). Default customer rate-limit reply: "I'm getting too many messages right now. Please try again in a minute."
- Guard block before model: fixed refusal "I can't help with that request.", `error.code = guard_blocked`, no tools.

### Catalog

- Tables: `skills` (`id`, `name` unique, `description`, `enabled`, `updated_at`); `tools` (`id`, `skill_id`, `name` unique, `description`, `method`, `endpoint`, `auth`, `timeout_ms`, `enabled`, `parameters` JSON Schema, `response` JSON, `requires` tool-name list, `updated_at`).
- Fixture: `order_support` with `get_order_status` then `refund_order` as in the foundation brief; `refund_order.requires` includes `get_order_status`.
- `auth` names a scheme (`bearer`). Credential from environment: `TOOL_BEARER_TOKEN` for all bearer tools.
- Secrets never stored in catalog tables.

### LLM

- Provider, model name, and temperature from config/environment.
- Two model calls: skill selection (no tool schemas); then reply with tools attached only for chosen skills.
- Transcript: `conversation_history` plus current `content`. History budget per request only: newest 12 turns, then drop older until token budget fits; summarize dropped prefix in memory for this call only; do not persist. Service has no conversation database.
- Skill-selection and tool-calling prompts as in the foundation brief, plus the explicit-request rule on the tool-calling prompt.
- Selection returns empty `skill_names` → general answer path. Selection decode failure → same.
- Hallucination metric: reply asserts an order id, refund id, or other kept-field value that no successful tool result contained. Publish still happens.

### Resilience and observability

- Per-message timeout budget (configurable; grilling proposed 30s as default, not yet locked in Further Notes).
- Circuit breaker per tool endpoint; retry with backoff and jitter inside remaining budget.
- Structured logs: `message_id`, `conversation_id`, `trace_id`. One OpenTelemetry trace per attempt.
- Metrics: end-to-end latency, tool-call success rate, token usage, hallucination count.
- Small `net/http` server for health and metrics beside the consumer loop.

### Language and layout

- Go module. Project structure supports: Kafka consumer/producer, catalog repository, attempt orchestration, model client, tool executor, health/metrics.
- Unit tests per stage behind the attempt seam; contract tests for input/output schemas; end-to-end test with tools and model mocked through adapters.
- Dockerfile, environment variables, and notes for adding consumer replicas.

### Kafka topic and group names (locked)

| Name | Value |
| --- | --- |
| Input | `chat.customer.messages` |
| Output | `chat.agent.replies` |
| DLQ | `chat.customer.messages.dlq` |
| Consumer group | `agentic-loop` |

## Testing Decisions

- A good test exercises external behaviour through the attempt seam: given one message (and adapter behaviour), assert the output or DLQ record and which tool requests ran.
- Do not assert on private ReAct step counts, prompt string equality beyond required contracts, or internal function call graphs.
- Prefer the highest seam: the attempt. Kafka, Postgres, HTTP clients, and the model provider are adapters with fakes in tests.
- Contract tests validate input and output JSON against the Go structs / schemas.
- End-to-end test: fixture catalog (`order_support`), mocked model (selection then tool calls), mocked order API; cover happy path status+refund, no skill match, required-read refusal, duplicate `message_id`, rate limit, guard block, tool failure → `tool_failed`.
- No prior art in-repo yet; this is greenfield.

## Out of Scope

- Legacy chat UI, conversation storage, or writes to legacy databases.
- Redis or a second shared store.
- Confirmation turns before write tools.
- Executor-side classification of “explicit request” or order-status eligibility policy.
- Per-tool or per-skill credential vault beyond one env bearer token.
- Calibrated confidence / probability models.
- Loading the full skill catalog into memory or into the prompt.
- Separate plan-and-execute mode alongside ReAct.
- Persisting conversation summaries or chat history in this service.
- Settling the open grilling items listed under Further Notes as product requirements until they are accepted.

## Further Notes

### Accepted grilling (rounds 1–2) — already in ADRs / decisions above

Explicit write request; no-skill general answer; one stored reply per `message_id`; executor `requires`; Kafka names; Postgres for dedupe/rate limits; worker/model/agent naming; account facts from tools; `tool_failed` when requested account fact missing; hallucination string check; history trim with fixed omission line (proposed detail: 12 turns / 3000 tokens — treat 12 as locked from round 2; 3000 tokens as the proposed budget); 30s message budget / 90s claim (proposed in round 2 Q4 — treat as recommended defaults); max four tool executions; executor ignores status; prompt enforces explicit request; `customer_id` from message; one bearer token; 10/min rate limit; fixed guard refusal.

### Open grilling (round 3) — proposed, not accepted

Do not implement as locked until accepted. Recommendations were:

- Worker-assigned confidence ladder (`0` / `0.2` / `0.4` / `0.8`, hallucination caps at `0.2`).
- Model failure / empty-evidence template: "I can't answer that right now." with `no_reply` or `timeout`; template from successful tool fields without a third model call.
- Stopping at four tools with a missing fact → `tool_failed` (vs `timeout` when budget stops the attempt).
- Record write tool name+args on the claim immediately before HTTP; later attempts skip those writes.
- Store → publish → commit Kafka offset only after publish succeeds.
- Read retries (twice on timeout/429/5xx); no write retries; circuit breaker 5 failures / 30s open.
- Output safety: replace reply with guard sentence and `guard_blocked` if it matches input guard patterns.
- Input guard: fixed pattern list, no classifier model or moderation API.
- One model id from `MODEL`, temperature `0` for both calls.
- Malformed input: DLQ then commit offset; may DLQ again on redelivery.
- Fixed English customer sentences as proposed in round 3 Q11.

### Foundation deliverables checklist

Architecture Mermaid; Go input/output structs; Postgres schema + top-20 search + fixture; attempt orchestration; Go module layout; Kafka consumer/producer with DLQ and dedupe; prompts wired (tools only on second call); error-handling matrix; Dockerfile + env + scaling notes; minimal runnable skeleton.
