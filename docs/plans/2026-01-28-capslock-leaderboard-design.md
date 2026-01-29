# Capslock Leaderboard Design

## Overview

A Twitch chat monitoring feature that tracks users who post entirely in caps and maintains a "loudest chatters" leaderboard per channel. Designed for entertainment and community engagement.

## Requirements

### Scoring Rules
- **Detection**: Entire message must be all caps (strict)
- **Minimum**: Message must contain at least 2 words
- **Scoring**: Simple count — each qualifying message = +1 point

### Leaderboard Features
- **Scope**: Per-channel only (each streamer has their own leaderboard)
- **Timeframes**: All-time, monthly, and daily
- **Access**: Channel owners can view and display leaderboards
- **Display**: Both chat command and web page for stream overlays

### Connection Model
- Always-on persistent IRC connections to configured channels
- Scores update in real-time as messages arrive

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────┐
│  Twitch IRC     │────▶│  Chat Monitor    │────▶│   SQLite    │
│  (per channel)  │     │  Service (Go)    │     │   (scores)  │
└─────────────────┘     └──────────────────┘     └─────────────┘
                                                        │
                        ┌──────────────────┐            │
                        │   HTTP Handlers  │◀───────────┘
                        │  - /api/caps     │
                        │  - web pages     │
                        └──────────────────┘
```

## Components

### Chat Monitor Service

A goroutine-based service that runs alongside the existing HTTP server.

**Responsibilities:**
- Maintain persistent IRC connections to Twitch channels
- Parse incoming PRIVMSG events
- Validate messages against scoring rules (all caps, 2+ words)
- Record qualifying messages to the database

**IRC Connection Details:**
- Server: `irc.chat.twitch.tv:6697` (TLS)
- Authentication: OAuth token with `chat:read` scope
- One connection can join multiple channels (up to 20 per connection recommended)
- Handle PING/PONG for keepalive
- Reconnect logic for dropped connections

**Message Validation:**
```go
func isQualifyingCapsMessage(msg string) bool {
    // Remove non-letter characters for caps check
    letters := extractLetters(msg)
    if len(letters) == 0 {
        return false
    }

    // Check all letters are uppercase
    if letters != strings.ToUpper(letters) {
        return false
    }

    // Check minimum 2 words
    words := strings.Fields(msg)
    return len(words) >= 2
}
```

### Database Schema

**New table: `caps_events`**
```sql
CREATE TABLE caps_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel TEXT NOT NULL,
    username TEXT NOT NULL,
    display_name TEXT NOT NULL,
    message_length INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_caps_events_channel ON caps_events(channel);
CREATE INDEX idx_caps_events_channel_created ON caps_events(channel, created_at);
CREATE INDEX idx_caps_events_username ON caps_events(channel, username);
```

**Leaderboard queries:**
```sql
-- All-time leaderboard
SELECT username, display_name, COUNT(*) as score
FROM caps_events
WHERE channel = ?
GROUP BY username
ORDER BY score DESC
LIMIT 10;

-- Daily leaderboard
SELECT username, display_name, COUNT(*) as score
FROM caps_events
WHERE channel = ?
  AND created_at >= date('now', 'start of day')
GROUP BY username
ORDER BY score DESC
LIMIT 10;

-- Monthly leaderboard
SELECT username, display_name, COUNT(*) as score
FROM caps_events
WHERE channel = ?
  AND created_at >= date('now', 'start of month')
GROUP BY username
ORDER BY score DESC
LIMIT 10;
```

### API Endpoints

**GET `/api/channels/{channel}/capsboard`**

Query parameters:
- `period`: `daily` | `monthly` | `alltime` (default: `alltime`)
- `limit`: Number of entries (default: 10, max: 50)

Response (JSON):
```json
{
  "channel": "streamer_name",
  "period": "alltime",
  "leaderboard": [
    {"rank": 1, "username": "loud_user", "display_name": "LOUD_USER", "score": 147},
    {"rank": 2, "username": "caps_fan", "display_name": "Caps_Fan", "score": 89}
  ],
  "updated_at": "2026-01-28T15:30:00Z"
}
```

Response (plain text, for Nightbot):
```
Capslock Leaderboard: 1. LOUD_USER (147) 2. Caps_Fan (89) 3. YellingGuy (56)
```

Content negotiation via `Accept` header or `Nightbot-Channel` header presence.

### Web Page

**GET `/channels/{channel}/capsboard`**

HTML page suitable for OBS browser source:
- Dark theme by default (stream-friendly)
- Auto-refresh every 30 seconds
- Displays top 10 for selected timeframe
- Tab/button toggle between daily/monthly/all-time
- Minimal, clean design with large readable text

Optional query params:
- `?period=daily|monthly|alltime` — Set initial view
- `?theme=light|dark` — Override theme
- `?refresh=N` — Custom refresh interval in seconds

### Chat Command Integration

Nightbot command setup:
```
!capsboard → $(urlfetch https://quoteqt.example.com/api/channels/$(channel)/capsboard?period=alltime)
!capstoday → $(urlfetch https://quoteqt.example.com/api/channels/$(channel)/capsboard?period=daily)
```

## Configuration

**New config options:**
```go
type CapsMonitorConfig struct {
    Enabled       bool     `json:"enabled"`
    Channels      []string `json:"channels"`       // Channels to monitor
    TwitchOAuth   string   `json:"twitch_oauth"`   // OAuth token for IRC
    MinWords      int      `json:"min_words"`      // Minimum words (default: 2)
}
```

**Channel management:**
- Channels can be added/removed via admin API or config
- Each channel in `channels` list gets monitored
- Channel owners (from existing `channel_owners` table) can view their leaderboards

## Implementation Phases

### Phase 1: Core Infrastructure
- IRC connection manager with reconnect logic
- Message parsing and validation
- Database schema and migrations
- Basic caps_events recording

### Phase 2: API and Display
- Leaderboard API endpoint with period filtering
- Plain text response for Nightbot
- Web page with auto-refresh

### Phase 3: Admin and Polish
- Admin UI for enabling/disabling monitoring per channel
- Channel owner access controls
- OBS-optimized styling for web page

## Future Considerations (Out of Scope)

- Global cross-channel leaderboard
- Achievements/badges for milestones
- "Caps streak" tracking (consecutive caps messages)
- Discord integration
- Character count as secondary metric
