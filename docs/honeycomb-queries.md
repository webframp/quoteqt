# Honeycomb Query Examples

This document shows how to analyze quoteqt API usage in Honeycomb, with a focus on breaking down data by streamer channel.

## Key Fields for Streamer Analysis

| Field | Description | Example Values |
|-------|-------------|----------------|
| `nightbot.channel.name` | Streamer's channel name | `beastyqt`, `marinelord` |
| `nightbot.channel.provider` | Platform | `twitch`, `youtube` |
| `nightbot.channel.provider_id` | Platform-specific ID | `12345678` |
| `nightbot.user.name` | Viewer who triggered command | `viewer123` |
| `nightbot.user.user_level` | Viewer's role | `owner`, `moderator`, `regular` |

## Query Examples

### 1. Request Count by Streamer

See which streamers are using the bot most:

```
VISUALIZE: COUNT
GROUP BY: nightbot.channel.name
WHERE: nightbot.channel.name exists
```

### 2. Request Rate by Streamer Over Time

Track usage patterns throughout a stream:

```
VISUALIZE: COUNT
GROUP BY: nightbot.channel.name
WHERE: nightbot.channel.name exists
```
(Use time picker to select relevant time range)

### 3. Error Rate by Streamer

Identify if certain streamers experience more issues:

```
VISUALIZE: COUNT
GROUP BY: nightbot.channel.name, http.response.status_code
WHERE: nightbot.channel.name exists
```

### 4. Rate Limited Requests by Streamer

See which channels are hitting rate limits:

```
VISUALIZE: COUNT
WHERE: name = "rate_limited"
GROUP BY: rate_limit.key
```

Or using status code:
```
VISUALIZE: COUNT
WHERE: http.response.status_code = 429
GROUP BY: nightbot.channel.name
```

### 5. Latency (P95) by Streamer

Check if certain channels have slower response times:

```
VISUALIZE: P95(duration_ms)
GROUP BY: nightbot.channel.name
WHERE: nightbot.channel.name exists
```

### 6. Most Requested Civilizations by Streamer

See what civs each streamer's viewers ask about:

```
VISUALIZE: COUNT
GROUP BY: nightbot.channel.name, civ
WHERE: civ exists
```

### 7. Quote vs Matchup Usage by Streamer

Compare endpoint usage patterns:

```
VISUALIZE: COUNT
GROUP BY: nightbot.channel.name, url.path
WHERE: nightbot.channel.name exists
```

### 8. Platform Breakdown (Twitch vs YouTube)

```
VISUALIZE: COUNT
GROUP BY: nightbot.channel.provider
WHERE: nightbot.channel.provider exists
```

### 9. User Level Distribution

See if mostly mods or regular viewers use commands:

```
VISUALIZE: COUNT
GROUP BY: nightbot.user.user_level
WHERE: nightbot.user.user_level exists
```

### 10. No Results Rate by Streamer

Find channels that might need more quotes added:

```
VISUALIZE: COUNT
WHERE: name = "no_results"
GROUP BY: nightbot.channel.name
```

## Creating a Streamer Dashboard

Consider creating a Board with these queries:

1. **Total Requests** - COUNT over time
2. **Requests by Channel** - COUNT grouped by `nightbot.channel.name`
3. **P95 Latency** - P95(duration_ms) over time
4. **Error Rate** - COUNT where `http.response.status_code >= 400`
5. **Rate Limited %** - Calculated from 429 responses
6. **Top Civs Requested** - COUNT grouped by `civ`

## Filtering to a Single Streamer

Add this WHERE clause to any query to filter to one streamer:

```
WHERE: nightbot.channel.name = "beastyqt"
```

## Trace Exploration

To see individual requests from a streamer:

1. Query: `WHERE nightbot.channel.name = "beastyqt"`
2. Click on a trace to see the waterfall view
3. Child spans show database query timing
4. Span events show rate limiting, no results, etc.
