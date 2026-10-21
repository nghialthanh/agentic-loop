## Agent skills

### Repo layout

Three layers: `internal/repository` (DAO), `internal/service` (logic), `internal/adapter` (Kafka / HTTP / API). See `docs/structure.md`. Deploy/env: `docs/deploy.md`. Error matrix: `docs/error-handling.md`.

### Issue tracker

Issues live as markdown files under `.scratch/<feature>/`. See `docs/agents/issue-tracker.md`.

### Triage labels

Default triage roles: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
