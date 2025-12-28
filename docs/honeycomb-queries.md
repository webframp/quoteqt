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

To create these queries in the Honeycomb UI:

1. Go to your dataset → **New Query**
2. Set the **time range** in the top right
3. Add **WHERE** clauses by clicking "+ Add filter" 
4. Add **VISUALIZE** calculations
5. Add **GROUP BY** fields

### Error Rate by Endpoint

| Section | Value |
|---------|-------|
| VISUALIZE | COUNT |
| VISUALIZE | COUNT where `http.response.status_code` >= 500 |
| WHERE | `url.path` starts with `/api/` |
| GROUP BY | `url.path` |
| Time | Last 1 hour |

### Latency Percentiles

| Section | Value |
|---------|-------|
| VISUALIZE | P50(`duration_ms`) |
| VISUALIZE | P95(`duration_ms`) |
| VISUALIZE | P99(`duration_ms`) |
| WHERE | `url.path` starts with `/api/` |
| GROUP BY | `url.path` |
| Time | Last 1 hour |

### Nightbot Usage by Channel

| Section | Value |
|---------|-------|
| VISUALIZE | COUNT |
| WHERE | `nightbot.channel.name` exists |
| GROUP BY | `nightbot.channel.name` |
| Time | Last 24 hours |

### Slowest Requests

| Section | Value |
|---------|-------|
| VISUALIZE | MAX(`duration_ms`) |
| VISUALIZE | HEATMAP(`duration_ms`) |
| WHERE | `url.path` starts with `/api/` |
| GROUP BY | `url.path` |
| ORDER BY | MAX(`duration_ms`) DESC |
| Time | Last 1 hour |

### Database Query Performance

| Section | Value |
|---------|-------|
| VISUALIZE | P50(`duration_ms`) |
| VISUALIZE | P99(`duration_ms`) |
| VISUALIZE | COUNT |
| WHERE | `db.system` = `sqlite` |
| GROUP BY | `db.operation` |
| Time | Last 1 hour |

### Errors with Stack Traces

| Section | Value |
|---------|-------|
| VISUALIZE | COUNT |
| WHERE | `exception.message` exists |
| GROUP BY | `exception.message` |
| GROUP BY | `exception.type` |
| Time | Last 24 hours |

## Span Attributes

### HTTP Spans (from otelhttp)

| Attribute | Description |
|-----------|-------------|
| `http.request.method` | GET, POST, etc. |
| `url.path` | Request path |
| `http.response.status_code` | Response status code |
| `url.scheme` | http or https |
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
