# BLOCKED: savepoint fix does not apply to storefront-bff

The aborted-transaction bug (25P02 on the idempotency outcome UPDATE) needs a
Postgres transaction shared between the Idempotency-Key middleware and the
handler. storefront-bff has neither.

Evidence (checked on branch feature/savepoint-fix, cut from origin/develop):
- `internal/adapters/inbound/http/idempotency.go` holds only `requireKey`, a
  header validator. It stores nothing: the upstream key is minted by
  `composition.MintKey` and replays/409s come from the upstream provider.
- No pgx, database/sql, lib/pq or testcontainers in go.mod or in any non-architecture `.go` file.
- ADR 0001 ("No database"), ADR 0002 A8, and `.claude/rules/domain-model.md`
  say the BFF owns no database and no aggregates.

The task's invariant ("a mapped 4xx over an aborted tx is still recorded and
returned; a 2xx over an aborted tx is never returned or cached") holds
vacuously: there is no transaction to abort and no cache to poison.

Not done, deliberately: adding a Postgres dependency, a key table or a
transactional middleware would reverse ADR 0001/0002, change middleware
behaviour beyond the savepoint fix (forbidden), and need a user decision plus
an ADR. The poison tests cannot be ported (no `pgtx`, no Postgres fixture).

Suggested next step: drop storefront-bff from the savepoint rollout list.
Reopen only if an ADR introduces BFF-side idempotency storage.
