# 03: Skill catalog and top-20 candidates

**What to build:** Postgres holds skills and tools (including `requires` on tools). The `order_support` fixture with `get_order_status` and `refund_order` is loadable. Enabled skills are searchable by name and description to a top-20 candidate list. Enabled tools for chosen skills can be loaded without loading the full catalog into the prompt.

**Blocked by:** 01 (Module, contracts, and attempt seam)

**Status:** ready-for-agent

- [ ] Skills and tools tables exist with the fields from the spec (including `requires`)
- [ ] Fixture seeds `order_support` with status then refund; refund requires status
- [ ] Full-text search returns at most 20 enabled skills by name and description
- [ ] Disabled skills and tools are excluded from search and tool load
- [ ] Tools load only for named skills after selection
- [ ] Repository tests cover search and fixture load
