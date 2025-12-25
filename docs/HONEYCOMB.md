# Honeycomb Observability Guide

This application sends OpenTelemetry traces to Honeycomb for observability.

## Setup

1. Get API key from https://ui.honeycomb.io/account
2. Create `.env` file:
   ```
   HONEYCOMB_API_KEY=your-api-key
   OTEL_SERVICE_NAME=quoteqt
   ```
3. Restart the service: `sudo systemctl restart quotes`

## Available Attributes

### Standard HTTP Attributes (automatic via otelhttp)

| Attribute | Description | Example |
|-----------|-------------|---------|
| `http.method` | HTTP method | `GET` |
| `http.route` | Route pattern | `/api/quote` |
| `http.status_code` | Response status | `200` |
| `http.target` | Full path with query | `/api/quote?civ=hre` |
| `http.request_content_length` | Request body size | `0` |
| `http.response_content_length` | Response body size | `45` |
| `net.host.name` | Server hostname | `localhost` |
| `net.host.port` | Server port | `8000` |

### Nightbot Attributes (on API requests)

When requests come from Nightbot, these attributes are added:

| Attribute | Description | Example |
|-----------|-------------|---------|
| `nightbot.channel.name` | Channel username | `nightdev` |
| `nightbot.channel.display_name` | Channel display name | `NightDev` |
| `nightbot.channel.provider` | Streaming platform | `twitch`, `youtube` |
| `nightbot.channel.provider_id` | Platform channel ID | `11785491` |
| `nightbot.user.name` | User who ran command | `viewer123` |
| `nightbot.user.display_name` | User display name | `Viewer123` |
| `nightbot.user.provider` | User's platform | `twitch` |
| `nightbot.user.user_level` | Permission level | `owner`, `moderator`, `regular` |

## Example Queries

### Top Streamers Using the API

```
VISUALIZE: COUNT
GROUP BY: nightbot.channel.display_name
WHERE: nightbot.channel.name EXISTS
ORDER BY: COUNT DESC
```

### Requests by Platform (Twitch vs YouTube)

```
VISUALIZE: COUNT
GROUP BY: nightbot.channel.provider
WHERE: nightbot.channel.provider EXISTS
```

### API Endpoint Usage

```
VISUALIZE: COUNT
GROUP BY: http.route
WHERE: http.route STARTS_WITH "/api/"
```

### Slow Requests (p95 latency by endpoint)

```
VISUALIZE: P95(duration_ms)
GROUP BY: http.route
```

### Error Rate by Endpoint

```
VISUALIZE: COUNT
GROUP BY: http.route, http.status_code
WHERE: http.status_code >= 400
```

### Unique Streamers Per Day

```
VISUALIZE: COUNT_DISTINCT(nightbot.channel.name)
GROUP BY: $timestamp (1 day)
WHERE: nightbot.channel.name EXISTS
```

### Most Active Users (who triggers commands most)

```
VISUALIZE: COUNT
GROUP BY: nightbot.user.display_name, nightbot.channel.display_name
WHERE: nightbot.user.name EXISTS
ORDER BY: COUNT DESC
```

### Requests by Civilization Filter

```
VISUALIZE: COUNT
GROUP BY: http.target
WHERE: http.target CONTAINS "civ="
```

## Heatmap: Request Volume Over Time

```
VISUALIZE: HEATMAP(duration_ms)
WHERE: http.route STARTS_WITH "/api/"
```

## Setting Up Alerts

### High Error Rate Alert
- Query: `COUNT WHERE http.status_code >= 500`
- Threshold: > 10 in 5 minutes
- Useful for detecting outages

### Latency Degradation Alert  
- Query: `P95(duration_ms) WHERE http.route = "/api/quote"`
- Threshold: > 500ms
- Useful for detecting performance issues

## Dashboard Suggestions

Create a dashboard with these widgets:

1. **Request Volume** - Timeseries of total requests
2. **Error Rate** - Percentage of 4xx/5xx responses
3. **P95 Latency** - Response time trends
4. **Top Streamers** - Bar chart of most active channels
5. **Platform Split** - Pie chart of Twitch vs YouTube
6. **Endpoint Breakdown** - Table of requests per route

## Troubleshooting

### No Data in Honeycomb

1. Check API key is set: `grep HONEYCOMB /home/exedev/quotes/.env`
2. Check service logs: `journalctl -u quotes -n 50`
3. Look for "OpenTelemetry configured" on startup
4. Verify no "401 Unauthorized" errors

### Missing Nightbot Attributes

Nightbot attributes only appear when:
- Request includes `Nightbot-Channel` header
- Request is to `/api/*` endpoints

Test with:
```bash
curl -H "Nightbot-Channel: name=test&displayName=Test&provider=twitch&providerId=123" \
  https://grove-extra.exe.xyz:8000/api/quote
```
