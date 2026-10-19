# 01: Module, contracts, and attempt seam

**What to build:** A Go worker module with Kafka input/output structs that match the foundation contract, an attempt seam that accepts one message and returns one output or dead-letter result through fakes, a health HTTP endpoint, and contract tests for valid payloads.

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] Go module exists and builds
- [ ] Input and output message structs match the foundation schemas
- [ ] Attempt seam is callable with fake catalog, model, tools, store, and clock
- [ ] A valid message through the seam returns a well-formed output record (stub reply is enough)
- [ ] Health endpoint responds successfully
- [ ] Contract tests cover valid input and output shapes
