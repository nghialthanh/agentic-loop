# 08: Guard, tool failure, and loop budget

**What to build:** Input guard blocks (pattern list) produce the fixed refusal with `guard_blocked` and no model or tools. When a requested account fact is missing after a tool failure inside the message budget, `error.code` is `tool_failed` and the reply invents no missing facts. The loop stops at four tools; when the message timeout budget is spent, `error.code` is `timeout`. Use recommended defaults from the spec Further Notes (30s budget, pattern-list guard) unless superseded later.

**Blocked by:** 05 (Read tool loop and grounded reply)

**Status:** resolved

- [x] Guard match stores and returns `guard_blocked` with the fixed refusal
- [x] Missing requested account fact after tool failure sets `tool_failed`
- [x] Reply on tool failure does not guess missing account facts
- [x] Four-tool stop with a missing fact sets `tool_failed` when budget remains
- [x] Spent message budget publishes `timeout` and a short reply
- [x] Tests cover guard, tool failure, and timeout through the attempt seam

## Answer

Attempt seam now runs a fixed pattern-list input guard (refusal + `guard_blocked`, no model/tools), tracks the 30s message budget by deducting tool latency (`timeout` + short reply when spent), and sets `tool_failed` when a requested account fact is still missing after tool failure or a four-tool stop with budget remaining—without inventing missing facts. Covered by attempt-seam tests in `guard_budget_test.go`.
