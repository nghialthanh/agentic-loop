# 07: Claim, one stored reply, rate limit

**What to build:** Before any tool call, the worker claims `message_id` in shared Postgres. The winner stores the output then publishes that stored record. A redelivery publishes the stored reply and does not call tools. Per-customer rate limit (default 10 per rolling minute) yields a fixed reply with `error.code = rate_limited` and no model or tools; only the claim winner counts; redelivery does not count again. Claims expire so a crashed attempt can be taken over.

**Blocked by:** 05 (Read tool loop and grounded reply)

**Status:** resolved

- [x] Claim is taken before any tool HTTP call
- [x] Output is stored before publish; redelivery republishes without tools
- [x] Concurrent workers wait or take over only after claim expiry
- [x] Over-limit customer gets the fixed rate-limit reply and `rate_limited`
- [x] Rate limit and claims live in the same Postgres as the catalog
- [x] Tests cover duplicate delivery and rate limit through the attempt seam

## Answer

Attempt seam claims via `Store.Claim` before model/tools; winner runs the attempt, `SaveOutput`, then returns that stored record. Redelivery returns `ClaimStored` and skips model/tools. Live claims return `ErrClaimHeld`; expired claims are takeable. Per-customer rolling rate limit (default 10/min) stores the fixed reply with `error.code = rate_limited` and no model/tools; only the winner calls `ReserveRateLimit`; redelivery republishes without recounting. Postgres adapter + schema live in `internal/store` (same DB as catalog). Covered by attempt-seam tests in `claim_test.go` and store Postgres tests.
