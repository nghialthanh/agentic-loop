# 06: Write tools with requires and customer_id

**What to build:** An explicit refund request can call `get_order_status` then `refund_order`. The executor refuses a write when required reads have not succeeded in this attempt (no HTTP). `customer_id` is omitted from the model schema and bound from the message. The tool-calling prompt states that write tools need an explicit request in the latest message. The executor does not interpret order status for eligibility.

**Blocked by:** 05 (Read tool loop and grounded reply)

**Status:** ready-for-agent

- [ ] Refund after successful status read can complete when the model requests it
- [ ] Calling refund without a prior successful required read records tool error and makes no HTTP call
- [ ] `customer_id` on tool parameters is taken from the message, not the model
- [ ] Tool-calling prompt includes the explicit-request rule
- [ ] Executor does not refuse a write based on status string values
- [ ] Tests assert which HTTP calls ran through the attempt seam
