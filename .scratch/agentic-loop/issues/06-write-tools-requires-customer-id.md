# 06: Write tools with requires and customer_id

**What to build:** An explicit refund request can call `get_order_status` then `refund_order`. The executor refuses a write when required reads have not succeeded in this attempt (no HTTP). `customer_id` is omitted from the model schema and bound from the message. The tool-calling prompt states that write tools need an explicit request in the latest message. The executor does not interpret order status for eligibility.

**Blocked by:** 05 (Read tool loop and grounded reply)

**Status:** resolved

- [x] Refund after successful status read can complete when the model requests it
- [x] Calling refund without a prior successful required read records tool error and makes no HTTP call
- [x] `customer_id` on tool parameters is taken from the message, not the model
- [x] Tool-calling prompt includes the explicit-request rule
- [x] Executor does not refuse a write based on status string values
- [x] Tests assert which HTTP calls ran through the attempt seam

## Answer

Attempt seam enforces `requires` before HTTP (failed read → `used_tools` error, no call), binds `customer_id` from the message while omitting it from model schemas, and extends the tool-calling prompt with the explicit-write-request rule. Successful required reads alone gate writes; status strings are not interpreted. Covered by attempt-seam tests that assert which order-API HTTP calls ran.
