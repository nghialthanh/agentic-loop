# 01: Module, contracts, and attempt seam

**What to build:** A Go worker module with Kafka input/output structs that match the foundation contract, an attempt seam that accepts one message and returns one output or dead-letter result through fakes, a health HTTP endpoint, and contract tests for valid payloads.

**Blocked by:** None (can start immediately).

**Status:** resolved

- [x] Go module exists and builds
- [x] Input and output message structs match the foundation schemas
- [x] Attempt seam is callable with fake catalog, model, tools, store, and clock
- [x] A valid message through the seam returns a well-formed output record (stub reply is enough)
- [x] Health endpoint responds successfully
- [x] Contract tests cover valid input and output shapes

## Answer

Stub worker skeleton: foundation Kafka input/output structs in `internal/contract`, attempt seam with adapter deps and stub reply in `internal/attempt`, `GET /health` in `internal/health`, plus contract/attempt/health tests. Module builds; valid message through the seam returns a well-formed output.
