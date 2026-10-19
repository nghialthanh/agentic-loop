# 09: Kafka consumer and publisher

**What to build:** The worker consumes `chat.customer.messages` in group `agentic-loop`, runs the attempt, and publishes to `chat.agent.replies` or `chat.customer.messages.dlq` with key `message_id`. Valid paths and failure paths from earlier tickets work end-to-end over Kafka. Offset commit follows store-then-publish success for accepted messages (recommended behaviour from Further Notes).

**Blocked by:** 02 (Malformed input to dead letter), 06 (Write tools with requires and customer_id), 07 (Claim, one stored reply, rate limit), 08 (Guard, tool failure, and loop budget)

**Status:** ready-for-agent

- [ ] Consumer group `agentic-loop` reads the input topic
- [ ] Successful attempts publish to the output topic with key `message_id`
- [ ] Malformed input publishes to the DLQ topic
- [ ] Horizontal replicas share the group without conversation pinning
- [ ] Integration test covers at least one happy path and one DLQ path over Kafka
