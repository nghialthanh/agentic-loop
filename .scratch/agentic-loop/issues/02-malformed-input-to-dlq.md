# 02: Malformed input to dead letter

**What to build:** When a message fails schema validation, has empty content after trim, or uses an unknown history role, the attempt produces a dead-letter record with the original payload and reason, and does not produce a normal output or a dedupe claim.

**Blocked by:** 01 (Module, contracts, and attempt seam)

**Status:** resolved

- [x] Schema-invalid payloads go to DLQ only
- [x] Empty `content` after trim goes to DLQ only
- [x] Unknown `conversation_history[].role` goes to DLQ only
- [x] No normal output record is produced for malformed input
- [x] No dedupe/claim row is written for malformed input
- [x] Tests drive this through the attempt seam

## Answer

Attempt seam returns dead letters for empty content, unknown history roles, and schema-invalid JSON (`RunBytes` keeps the original payload). Validation runs before `Store.ClaimMessage`, so malformed input never produces output or a claim. Covered by attempt-seam tests with a recording store.
