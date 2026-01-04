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

- [ ] **Track command modifications** - Investigate Nightbot API for `updatedBy` or similar field to show who last modified each command
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
- [ ] **Search within snapshots** - Find commands across snapshots
- [ ] **Export diff as text** - Download diff output for sharing

## Low Priority / Future Ideas

- [ ] **Multi-user snapshot access** - Allow non-admin users to view (not modify) snapshots
- [ ] **Webhook notifications** - Notify on significant command changes
- [ ] **Command analytics** - Track which commands are most used/modified

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
