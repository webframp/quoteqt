# Honeycomb Observability

This document describes the observability setup for quoteqt using Honeycomb.

## Setup

### Prerequisites

1. Honeycomb account with API key
2. `HONEYCOMB_API_KEY` set in `.env`
3. Terraform (optional, for automated setup)

### Automated Setup with Terraform

```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your email

export HONEYCOMB_API_KEY="your-api-key"
terraform init
terraform plan
terraform apply
```

This creates:
- Derived columns for easier querying
- Saved queries for common investigations
- Alert triggers for error rate, latency, and downtime
- SLO for API availability (99.9%)

## Manual Queries

If you prefer to set things up manually in the Honeycomb UI, here are the key queries:

### Error Rate by Endpoint

```
VISUALIZE: COUNT, COUNT_DISTINCT(trace.trace_id) WHERE http.status_code >= 500
GROUP BY: http.target
FILTER: http.target starts with "/api/"
TIME: Last 1 hour
```

### Latency Percentiles

```
VISUALIZE: P50(duration_ms), P95(duration_ms), P99(duration_ms)
GROUP BY: http.target  
FILTER: http.target starts with "/api/"
TIME: Last 1 hour
```

### Nightbot Usage by Channel

```
VISUALIZE: COUNT
GROUP BY: nightbot.channel.name
FILTER: nightbot.channel.name exists
TIME: Last 24 hours
```

### Slowest Requests

```
VISUALIZE: MAX(duration_ms), HEATMAP(duration_ms)
GROUP BY: http.target
ORDER BY: MAX(duration_ms) DESC
TIME: Last 1 hour
```

### Database Query Performance

```
VISUALIZE: P50(duration_ms), P99(duration_ms), COUNT
GROUP BY: db.operation
FILTER: db.system = "sqlite"
TIME: Last 1 hour
```

### Errors with Stack Traces

```
VISUALIZE: COUNT
GROUP BY: exception.message, exception.type
FILTER: exception.message exists
TIME: Last 24 hours
```

## Span Attributes

### HTTP Spans (from otelhttp)

| Attribute | Description |
|-----------|-------------|
| `http.method` | GET, POST, etc. |
| `http.target` | Request path |
| `http.status_code` | Response status code |
| `http.host` | Request host |
| `duration_ms` | Request duration |

### Database Spans

| Attribute | Description |
|-----------|-------------|
| `db.system` | Always "sqlite" |
| `db.operation` | Query name (e.g., GetRandomQuote) |
| `quote.id` | Quote ID (when applicable) |
| `civ.input` | Civilization filter input |

### Nightbot Spans

| Attribute | Description |
|-----------|-------------|
| `nightbot.channel.name` | Streamer's channel name |
| `nightbot.channel.provider` | twitch or youtube |
| `nightbot.user.name` | User who triggered command |
| `nightbot.user.user_level` | owner, moderator, regular, etc. |

### Error Spans

| Attribute | Description |
|-----------|-------------|
| `exception.type` | Error type |
| `exception.message` | Error message |
| `exception.stacktrace` | Go stack trace |

## Recommended Alerts

| Alert | Condition | Frequency |
|-------|-----------|----------|
| High Error Rate | > 5% errors over 5 min | Every 5 min |
| High Latency | P99 > 1 second | Every 5 min |
| Zero Traffic | 0 requests in 15 min | Every 15 min |

## SLO

**API Availability**: 99.9% of `/api/*` requests should succeed (status < 500)

This gives a monthly error budget of ~43 minutes of downtime or equivalent error volume.
