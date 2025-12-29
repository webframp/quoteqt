# Honeycomb Markers Implementation

Add Honeycomb markers to track significant events in the application lifecycle.

## Overview

Markers appear as vertical lines on Honeycomb graphs, helping correlate behavior changes with known events like deploys, migrations, and configuration changes.

## Implementation Checklist

### Phase 1: Deploy Markers ✅

- [x] Create `markers.go` with Honeycomb API client
  - [x] Define marker types as constants
  - [x] Implement `CreateMarker()` function
  - [x] Handle missing API key gracefully (skip when not configured)
- [x] Add deploy marker on server startup
  - [x] Include git commit SHA if available (via build-time variable)
  - [x] Include link to GitHub commit
- [x] Test deploy marker creation
- [x] Update Makefile with ldflags for version/commit

### Phase 2: Migration Markers ✅

- [x] Modify migration runner to track timing
- [x] Create marker after each migration completes
  - [x] Include migration filename in message
  - [x] Use start/end time for duration visibility
- [x] Return migration results from RunMigrations()

### Phase 3: Configuration Change Markers ✅

- [x] Add marker when channel owner is added
- [x] Add marker when channel owner is removed
- [x] Include relevant details (channel, email) in message

### Phase 4: Bulk Operation Markers ✅

- [x] Add marker for bulk delete operations
- [x] Add marker for bulk channel assignment
- [x] Add marker for bulk civilization assignment
- [x] Include count and operation type in message

## API Details

**Endpoint:** `POST https://api.honeycomb.io/1/markers/{dataset}`

**Headers:**
- `X-Honeycomb-Team: <API_KEY>`
- `Content-Type: application/json`

**Body:**
```json
{
  "start_time": 1471040808,
  "end_time": 1471040920,      // optional, for time ranges
  "message": "Deploy v1.2.3",
  "type": "deploy",            // groups markers by color
  "url": "https://github.com/..."  // optional, clickable link
}
```

**Marker Types:**
- `deploy` - Application deployments
- `migration` - Database migrations
- `config-change` - Admin configuration changes
- `bulk-operation` - Bulk data modifications

## Environment Variables

Uses existing `HONEYCOMB_API_KEY` - markers are skipped if not set.

Dataset is `OTEL_SERVICE_NAME` (defaults to "quoteqt").
