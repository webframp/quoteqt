# TODO

## High Priority

### Test Coverage Improvements

Current coverage: ~36%. Priority areas to improve:

- [ ] **Nightbot handlers (0% coverage)** - All nightbot.go handlers are untested
  - HandleNightbotAdmin, HandleNightbotCallback, HandleNightbotExport
  - HandleNightbotImport, HandleNightbotSnapshots, HandleNightbotSnapshotDiff
  - HandleNightbotSnapshotCompare, HandleNightbotSnapshotDelete/Undelete
  - Consider mocking Nightbot API for unit tests

- [x] **Middleware (~98% coverage)** - Gzip, RequestLogger, SecurityHeaders, StaticFileServer

- [x] **Suggestion handlers** - HandleListSuggestions (84%), HandleRejectSuggestion (80%)

- [x] **Channel owner management** - HandleAddChannelOwner (80%), HandleRemoveChannelOwner (73%)

### Nightbot Feature Enhancements

- [x] **Track command modifications** - INVESTIGATED: Nightbot API does NOT provide `updatedBy` field.
  The API only returns `createdAt` and `updatedAt` timestamps, but not who made changes.
  This is a Nightbot API limitation - cannot be implemented without Nightbot adding this feature.
- [ ] **Scheduled snapshots** - Option to automatically take daily/weekly snapshots of connected channels
- [ ] **Snapshot notes editing** - Allow editing the note on existing snapshots
- [ ] **Bulk snapshot operations** - Delete multiple snapshots at once

## Medium Priority

### Code Quality

- [ ] **Extract Nightbot client** - nightbot.go is 1700+ lines; extract API client to separate package
- [ ] **Integration tests for Nightbot** - End-to-end tests with test Nightbot account
- [ ] **Improve bot detection tests** - AddBotAttributes only 21% covered

### UI/UX Improvements

- [ ] **Snapshot pagination** - Currently limited to 100 snapshots per channel
- [x] **Search within snapshots** - Find commands across snapshots (/admin/nightbot/search)
- [x] **Export diff as text** - "Copy for Discord" button on diff pages

## Low Priority / Future Ideas

- [ ] **Webhook notifications** - Notify on significant command changes
- [ ] **Command analytics** - Track which commands are most used/modified

## Role-Based Access Control (RBAC) Refactoring

Currently Nightbot features are admin-only. Need to extend to channel owners.

### Current Roles (Implicit)
| Role | Description |
|------|-------------|
| Admin | Full access to everything, defined in ADMIN_EMAILS env var |
| Channel Owner | Can manage quotes/suggestions for their channel(s) only |
| Anonymous | Public API access, can submit suggestions |

### Needed Changes

- [ ] **Define explicit permission sets** for each role
- [ ] **Nightbot access for channel owners** - Currently admin-only
  - Channel owners should see/manage only their own channel's Nightbot backups
  - Admins see all channels across the system
- [ ] **Associate Nightbot channels with channel owners**
  - Currently snapshots are tied to channel_name but not to owner relationships
  - Need to link nightbot_snapshots.channel_name to channel_owners table
  - Or allow channel owners to "claim" a Nightbot channel
- [ ] **Update handlers to check channel ownership**
  - HandleNightbotAdmin: filter channels by ownership
  - HandleNightbotSnapshots: verify user owns/admins the channel
  - HandleNightbotSearch: scope to owned channels (or all for admin)
  - All snapshot operations: verify ownership
- [ ] **UI changes**
  - Show Nightbot nav link to channel owners (currently admin-only)
  - Filter channel list based on role

### Implementation Notes
- The `canManageChannel(ctx, email, channel)` helper already exists for quotes
- Could reuse this pattern for Nightbot access
- Consider: should channel owners be able to OAuth connect, or only view imported snapshots?

## Completed Features

### Nightbot Backup System (Jan 2026)
- [x] OAuth connection for owned channels
- [x] Tampermonkey script for managed channels (docs/tampermonkey-nightbot-exporter.js)
- [x] Snapshot history with timestamps
- [x] Git-style unified diff view (snapshot vs live)
- [x] Snapshot-to-snapshot comparison
- [x] Restore snapshots to Nightbot
- [x] Cached diff summaries on snapshot list
- [x] API token auth for Tampermonkey imports
- [x] Import token display in admin UI with copy button
- [x] Multi-channel support per user
- [x] Soft delete with 14-day restore window
- [x] Auto-purge of old deleted snapshots

### Quote System
- [x] Random quote API with civ/matchup filtering
- [x] Bot detection (Nightbot, StreamElements, Fossabot)
- [x] Suggestion system with rate limiting
- [x] Channel owner management
- [x] Bulk quote import
