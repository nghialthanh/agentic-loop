# Dedupe and rate limits live in Postgres

Every replica must see the same dedupe record and the same per-customer rate limit. The skill catalog is already in Postgres, so dedupe and rate limits are further tables in that database. Redis was rejected so the first system has one store. Counters and claim expiry are SQL rather than Redis primitives.
