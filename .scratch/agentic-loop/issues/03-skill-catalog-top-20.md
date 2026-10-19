# 03: Skill catalog and top-20 candidates

**What to build:** Postgres holds skills and tools (including `requires` on tools). The `order_support` fixture with `get_order_status` and `refund_order` is loadable. Enabled skills are searchable by name and description to a top-20 candidate list. Enabled tools for chosen skills can be loaded without loading the full catalog into the prompt.

**Blocked by:** 01 (Module, contracts, and attempt seam)

**Status:** resolved

- [x] Skills and tools tables exist with the fields from the spec (including `requires`)
- [x] Fixture seeds `order_support` with status then refund; refund requires status
- [x] Full-text search returns at most 20 enabled skills by name and description
- [x] Disabled skills and tools are excluded from search and tool load
- [x] Tools load only for named skills after selection
- [x] Repository tests cover search and fixture load

## Answer

Postgres catalog in `internal/catalog`: migrate `skills`/`tools` (with `requires`), seed `order_support` (`get_order_status` then `refund_order` requiring status), `SearchSkills` (FTS top 20, enabled only), and `ToolsForSkills` (enabled tools for named skills only). Repository tests cover fixture load, search cap/exclusion, and tool load exclusion.
