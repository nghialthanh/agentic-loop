# 09: Kafka consumer and publisher

**What to build:** The worker consumes `chat.customer.messages` in group `agentic-loop`, runs the attempt, and publishes to `chat.agent.replies` or `chat.customer.messages.dlq` with key `message_id`. Valid paths and failure paths from earlier tickets work end-to-end over Kafka. Offset commit follows store-then-publish success for accepted messages (recommended behaviour from Further Notes).

**Blocked by:** 02 (Malformed input to dead letter), 06 (Write tools with requires and customer_id), 07 (Claim, one stored reply, rate limit), 08 (Guard, tool failure, and loop budget)

**Status:** resolved

- [x] Consumer group `agentic-loop` reads the input topic
- [x] Successful attempts publish to the output topic with key `message_id`
- [x] Malformed input publishes to the DLQ topic
- [x] Horizontal replicas share the group without conversation pinning
- [x] Integration test covers at least one happy path and one DLQ path over Kafka

## Answer

`internal/kafka` consumes `chat.customer.messages` as group `agentic-loop`, runs `attempt.RunBytes`, publishes to `chat.agent.replies` or `chat.customer.messages.dlq` with key `message_id`, then commits the input offset only after publish succeeds. Unit tests cover publish-then-commit ordering; testcontainers integration covers one happy path and one DLQ path over real Kafka. `cmd/worker` starts the consumer when `KAFKA_BROKERS` is set.
