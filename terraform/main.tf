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
  expression  = "IF(GTE($http.status_code, 500), true, EXISTS($exception.message))"
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
# Email Recipient
# ============================================================================

resource "honeycombio_email_recipient" "alerts" {
  address = var.notification_email
}

# ============================================================================
# Queries for Triggers
# ============================================================================

# Query specification for error rate trigger
data "honeycombio_query_specification" "high_error_rate" {
  calculation {
    op     = "SUM"
    column = "is_error"
  }

  filter {
    column = "http.target"
    op     = "starts-with"
    value  = "/api/"
  }

  time_range = 300
}

resource "honeycombio_query" "high_error_rate" {
  dataset    = var.dataset
  query_json = data.honeycombio_query_specification.high_error_rate.json
}

# Query specification for latency trigger
data "honeycombio_query_specification" "high_latency" {
  calculation {
    op     = "P99"
    column = "duration_ms"
  }

  filter {
    column = "http.target"
    op     = "starts-with"
    value  = "/api/"
  }

  time_range = 300
}

resource "honeycombio_query" "high_latency" {
  dataset    = var.dataset
  query_json = data.honeycombio_query_specification.high_latency.json
}

# Query specification for zero traffic trigger
data "honeycombio_query_specification" "zero_traffic" {
  calculation {
    op = "COUNT"
  }

  filter {
    column = "http.target"
    op     = "starts-with"
    value  = "/api/"
  }

  time_range = 900
}

resource "honeycombio_query" "zero_traffic" {
  dataset    = var.dataset
  query_json = data.honeycombio_query_specification.zero_traffic.json
}

# ============================================================================
# Triggers (Alerts)
# ============================================================================

resource "honeycombio_trigger" "high_error_rate" {
  dataset     = var.dataset
  name        = "High Error Rate"
  description = "Fires when error count exceeds threshold over 5 minutes"
  disabled    = false
  query_id    = honeycombio_query.high_error_rate.id
  frequency   = 300

  threshold {
    op    = ">"
    value = 10
  }

  recipient {
    id = honeycombio_email_recipient.alerts.id
  }
}

resource "honeycombio_trigger" "high_latency" {
  dataset     = var.dataset
  name        = "High P99 Latency"
  description = "Fires when P99 latency exceeds 1 second"
  disabled    = false
  query_id    = honeycombio_query.high_latency.id
  frequency   = 300

  threshold {
    op    = ">"
    value = 1000
  }

  recipient {
    id = honeycombio_email_recipient.alerts.id
  }
}

resource "honeycombio_trigger" "zero_traffic" {
  dataset     = var.dataset
  name        = "Zero Traffic"
  description = "Fires when no API requests for 15 minutes (service may be down)"
  disabled    = false
  query_id    = honeycombio_query.zero_traffic.id
  frequency   = 900

  threshold {
    op    = "<"
    value = 1
  }

  recipient {
    id = honeycombio_email_recipient.alerts.id
  }
}

# ============================================================================
# SLO (Service Level Objective)
# ============================================================================

resource "honeycombio_slo" "api_availability" {
  dataset           = var.dataset
  name              = "API Availability"
  description       = "99.9% of API requests should succeed"
  sli               = honeycombio_derived_column.is_error.alias
  target_percentage = 99.9
  time_period       = 30
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
