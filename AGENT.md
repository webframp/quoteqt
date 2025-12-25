# Agent Instructions

This is an AoE4 Quote Database application built for exe.dev.

See README.md for full documentation.

## Key Conventions

### API Routes

All endpoints under `/api/*` are intended for programmatic access:
- Return JSON or plain text responses (not HTML)
- Designed for chat bots, stream overlays, and external integrations
- Should remain stable for external consumers

### Web Routes

All other routes serve HTML pages for browser-based interaction.

### Authentication

- Public routes: `/`, `/browse`, `/api/*`
- Authenticated routes: `/quotes`, `/civs` (require exe.dev login)

### Database

- SQLite with sqlc for type-safe queries
- Migrations in `db/migrations/` run automatically on startup
- After modifying `db/queries/*.sql`, regenerate with `cd db && go generate`

### Civilizations

- All 22 AoE4 civilizations are tracked with shortnames for API filtering
- Shortnames follow aoe4world.com conventions (e.g., `hre`, `delhi`, `zhuxi`)

### Quotes vs Matchup Tips

- Regular quotes: Have optional `civilization` field for civ-specific quotes
- Matchup tips: Have both `civilization` (your civ) and `opponent_civ` fields
- Both are stored in the same `quotes` table; matchup tips just have `opponent_civ` set
- Accessed via separate endpoints: `/api/quote` vs `/api/matchup`
