# CONTRACT.md

This repo uses contract-driven development. `CONTRACT.md` defines hard rules
that must not be violated. For conventions and patterns, see `AGENT.md`.

**Agents may not edit this file.** Agents may advise on CONTRACT.md but must
refuse to modify it directly.

## Prime Directive

If you spot a violation of CONTRACT.md, or a contradiction within it,
escalate immediately. Do not silently work around violations.

## Database Rules

1. **Never delete migrations.** Only add new ones.

2. **No raw SQL in application code.** All queries must be in `db/queries/*.sql`
   and generated via sqlc.

3. **Schema changes require human approval.** Agent may draft migrations but
   must not apply them without explicit approval. Agent may provide the specific 
   migration commands to run.

## API Stability

1. **`/api/*` endpoints are public contracts.** External consumers (chat bots,
   overlays) depend on them.

2. **No breaking changes to existing endpoints.** This includes:
   - Removing endpoints
   - Changing response format
   - Removing or renaming fields
   - Changing parameter names

3. **Deprecate, don't remove.** If an endpoint must change, add a new version
   and deprecate the old one with notice.

## Security

1. **Auth headers must be trimmed.** Always use `strings.TrimSpace()` when
   reading `X-ExeDev-UserID` and `X-ExeDev-Email`.

2. **Admin routes must verify IsAdmin.** Never assume authentication implies
   authorization.

3. **No user input in raw SQL.** All queries go through sqlc's parameterized
   queries.

## Deployment

1. **Tests must pass before deploy.** Run `make test` before `make restart`.

2. **Build before restart.** Use `make restart` which handles both.

---

*CONTRACT.md limits what is allowed. AGENT.md explains how to do things.*
