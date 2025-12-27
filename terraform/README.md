# Honeycomb Terraform Configuration

This Terraform configuration sets up observability infrastructure in Honeycomb for quoteqt.

## What it creates

- **Derived columns**: `is_error`, `duration_ms`, `is_api_request`, `is_nightbot`
- **Saved queries**: Error rate, latency percentiles, Nightbot usage, throughput
- **Triggers (alerts)**:
  - High error rate (>5% over 5 min)
  - High latency (P99 >1s)
  - Zero traffic (no requests for 15 min)
- **SLO**: 99.9% API availability over 30 days

## Usage

```bash
# Set your Honeycomb API key
export HONEYCOMB_API_KEY="your-api-key"

# Copy and edit the variables file
cp terraform.tfvars.example terraform.tfvars
vim terraform.tfvars

# Initialize and apply
terraform init
terraform plan
terraform apply
```

## Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `dataset` | Honeycomb dataset name | `quoteqt` |
| `notification_email` | Email for alert notifications | (required) |

## Notes

- The dataset must already exist (created automatically when the app sends traces)
- You need a Honeycomb API key with configuration permissions
- Triggers will send email notifications; add more recipients in the Honeycomb UI if needed
