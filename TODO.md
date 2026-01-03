# TODO

## Nightbot Command Backup Enhancements

- [x] **Store command snapshots in database** - Save Nightbot commands locally to enable history/versioning
- [x] **Diff view** - Show comparison between saved snapshots and current Nightbot configuration
- [x] **Tampermonkey import** - Export commands from managed channels via browser script
- [ ] **Track command modifications** - Investigate Nightbot API for `updatedBy` or similar field to show who last modified each command

## Completed Features

### Nightbot Backup System
- OAuth connection for owned channels
- Tampermonkey script for managed channels (docs/tampermonkey-nightbot-exporter.js)
- Snapshot history with timestamps
- Git-style unified diff view
- Restore snapshots to Nightbot
- Cached diff summaries on snapshot list
- API token auth for Tampermonkey imports
