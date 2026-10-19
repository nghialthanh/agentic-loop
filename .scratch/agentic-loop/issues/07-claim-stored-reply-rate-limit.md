# 07: Claim, one stored reply, rate limit

**What to build:** Before any tool call, the worker claims `message_id` in shared Postgres. The winner stores the output then publishes that stored record. A redelivery publishes the stored reply and does not call tools. Per-customer rate limit (default 10 per rolling minute) yields a fixed reply with `error.code = rate_limited` and no model or tools; only the claim winner counts; redelivery does not count again. Claims expire so a crashed attempt can be taken over.

**Blocked by:** 05 (Read tool loop and grounded reply)

**Status:** ready-for-agent

- [ ] Claim is taken before any tool HTTP call
- [ ] Output is stored before publish; redelivery republishes without tools
- [ ] Concurrent workers wait or take over only after claim expiry
- [ ] Over-limit customer gets the fixed rate-limit reply and `rate_limited`
- [ ] Rate limit and claims live in the same Postgres as the catalog
- [ ] Tests cover duplicate delivery and rate limit through the attempt seam
