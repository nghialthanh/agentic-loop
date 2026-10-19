# 08: Guard, tool failure, and loop budget

**What to build:** Input guard blocks (pattern list) produce the fixed refusal with `guard_blocked` and no model or tools. When a requested account fact is missing after a tool failure inside the message budget, `error.code` is `tool_failed` and the reply invents no missing facts. The loop stops at four tools; when the message timeout budget is spent, `error.code` is `timeout`. Use recommended defaults from the spec Further Notes (30s budget, pattern-list guard) unless superseded later.

**Blocked by:** 05 (Read tool loop and grounded reply)

**Status:** ready-for-agent

- [ ] Guard match stores and returns `guard_blocked` with the fixed refusal
- [ ] Missing requested account fact after tool failure sets `tool_failed`
- [ ] Reply on tool failure does not guess missing account facts
- [ ] Four-tool stop with a missing fact sets `tool_failed` when budget remains
- [ ] Spent message budget publishes `timeout` and a short reply
- [ ] Tests cover guard, tool failure, and timeout through the attempt seam
