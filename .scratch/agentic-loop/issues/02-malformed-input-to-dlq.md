# 02: Malformed input to dead letter

**What to build:** When a message fails schema validation, has empty content after trim, or uses an unknown history role, the attempt produces a dead-letter record with the original payload and reason, and does not produce a normal output or a dedupe claim.

**Blocked by:** 01 (Module, contracts, and attempt seam)

**Status:** ready-for-agent

- [ ] Schema-invalid payloads go to DLQ only
- [ ] Empty `content` after trim goes to DLQ only
- [ ] Unknown `conversation_history[].role` goes to DLQ only
- [ ] No normal output record is produced for malformed input
- [ ] No dedupe/claim row is written for malformed input
- [ ] Tests drive this through the attempt seam
