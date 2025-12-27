terraform {
  required_providers {
    honeycombio = {
      source  = "honeycombio/honeycombio"
      version = "~> 0.44.0"
    }
  }
}

provider "honeycombio" {
  # Set HONEYCOMB_API_KEY environment variable
  # or use: api_key = var.honeycomb_api_key
}

variable "dataset" {
  description = "Honeycomb dataset name"
  type        = string
  default     = "quoteqt"
}

variable "notification_email" {
  description = "Email for alert notifications"
  type        = string
}

# ============================================================================
# Derived Columns
# ============================================================================

resource "honeycombio_derived_column" "is_error" {
  dataset     = var.dataset
  alias       = "is_error"
  description = "Request resulted in an error (5xx or exception)"
  expression  = "GTE($http.status_code, 500) || EXISTS($exception.message)"
}

resource "honeycombio_derived_column" "duration_ms" {
  dataset     = var.dataset
  alias       = "duration_ms"
  description = "Request duration in milliseconds"
  expression  = "MUL($duration_ms, 1)"  # duration_ms should already exist from otelhttp
}

resource "honeycombio_derived_column" "is_api_request" {
  dataset     = var.dataset
  alias       = "is_api_request"
  description = "Request is to /api/* endpoint"
  expression  = "STARTS_WITH($http.target, \"/api/\")"
}

resource "honeycombio_derived_column" "is_nightbot" {
  dataset     = var.dataset
  alias       = "is_nightbot"
  description = "Request is from Nightbot"
  expression  = "EXISTS($nightbot.channel.name)"
}

# ============================================================================
# Queries (for reference - these create saved queries)
# ============================================================================

resource "honeycombio_query" "error_rate" {
  dataset = var.dataset

  query_json = jsonencode({
    calculations = [
      { op = "COUNT" },
      { op = "COUNT", column = "is_error" }
    ]
    filters = [
      { column = "is_api_request", op = "=", value = true }
    ]
    breakdowns = ["http.target"]
    time_range = 3600
  })

  query_annotation {
    name        = "API Error Rate by Endpoint"
    description = "Shows error counts per API endpoint over the last hour"
  }
}

resource "honeycombio_query" "latency_percentiles" {
  dataset = var.dataset

  query_json = jsonencode({
    calculations = [
      { op = "P50", column = "duration_ms" },
      { op = "P95", column = "duration_ms" },
      { op = "P99", column = "duration_ms" },
      { op = "COUNT" }
    ]
    filters = [
      { column = "is_api_request", op = "=", value = true }
    ]
    breakdowns = ["http.target"]
    time_range = 3600
  })

  query_annotation {
    name        = "API Latency Percentiles"
    description = "P50/P95/P99 latency for API endpoints"
  }
}

resource "honeycombio_query" "nightbot_usage" {
  dataset = var.dataset

  query_json = jsonencode({
    calculations = [
      { op = "COUNT" }
    ]
    filters = [
      { column = "is_nightbot", op = "=", value = true }
    ]
    breakdowns = ["nightbot.channel.name"]
    time_range = 86400
  })

  query_annotation {
    name        = "Nightbot Usage by Channel"
    description = "Request counts per Nightbot channel over 24 hours"
  }
}

resource "honeycombio_query" "throughput" {
  dataset = var.dataset

  query_json = jsonencode({
    calculations = [
      { op = "RATE_SUM", column = "duration_ms" },
      { op = "COUNT" }
    ]
    filters = [
      { column = "name", op = "=", value = "HTTP GET" }
    ]
    time_range = 3600
    granularity = 60
  })

  query_annotation {
    name        = "Request Throughput"
    description = "Requests per minute over the last hour"
  }
}

# ============================================================================
# Triggers (Alerts)
# ============================================================================

resource "honeycombio_recipient" "email" {
  type   = "email"
  target = var.notification_email
}

resource "honeycombio_trigger" "high_error_rate" {
  dataset     = var.dataset
  name        = "High Error Rate"
  description = "Fires when error rate exceeds 5% over 5 minutes"
  disabled    = false

  query_json = jsonencode({
    calculations = [
      { op = "COUNT" },
      { op = "SUM", column = "is_error" }
    ]
    filters = [
      { column = "is_api_request", op = "=", value = true }
    ]
    time_range = 300
  })

  frequency = 300  # Check every 5 minutes

  threshold {
    op    = ">"
    value = 0.05  # 5% error rate
  }

  recipient {
    id = honeycombio_recipient.email.id
  }
}

resource "honeycombio_trigger" "high_latency" {
  dataset     = var.dataset
  name        = "High P99 Latency"
  description = "Fires when P99 latency exceeds 1 second"
  disabled    = false

  query_json = jsonencode({
    calculations = [
      { op = "P99", column = "duration_ms" }
    ]
    filters = [
      { column = "is_api_request", op = "=", value = true }
    ]
    time_range = 300
  })

  frequency = 300

  threshold {
    op    = ">"
    value = 1000  # 1000ms = 1 second
  }

  recipient {
    id = honeycombio_recipient.email.id
  }
}

resource "honeycombio_trigger" "zero_traffic" {
  dataset     = var.dataset
  name        = "Zero Traffic"
  description = "Fires when no API requests for 15 minutes (service may be down)"
  disabled    = false

  query_json = jsonencode({
    calculations = [
      { op = "COUNT" }
    ]
    filters = [
      { column = "is_api_request", op = "=", value = true }
    ]
    time_range = 900  # 15 minutes
  })

  frequency = 900

  threshold {
    op    = "<"
    value = 1
  }

  recipient {
    id = honeycombio_recipient.email.id
  }
}

# ============================================================================
# SLO (Service Level Objective)
# ============================================================================

resource "honeycombio_slo" "api_availability" {
  dataset         = var.dataset
  name            = "API Availability"
  description     = "99.9% of API requests should succeed"
  sli             = honeycombio_derived_column.is_error.alias
  target_per_million = 999000  # 99.9%
  time_period     = 30  # 30 days
}

# ============================================================================
# Outputs
# ============================================================================

output "slo_id" {
  description = "ID of the API availability SLO"
  value       = honeycombio_slo.api_availability.id
}

output "trigger_ids" {
  description = "IDs of the alert triggers"
  value = {
    high_error_rate = honeycombio_trigger.high_error_rate.id
    high_latency    = honeycombio_trigger.high_latency.id
    zero_traffic    = honeycombio_trigger.zero_traffic.id
  }
}
