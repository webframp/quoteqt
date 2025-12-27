.PHONY: init plan apply destroy fmt validate

# Initialize Terraform
init:
	terraform init

# Preview changes
plan: init
	terraform plan

# Apply changes
apply: init
	terraform apply

# Apply without confirmation (use with caution)
apply-auto: init
	terraform apply -auto-approve

# Destroy all resources
destroy:
	terraform destroy

# Format terraform files
fmt:
	terraform fmt

# Validate configuration
validate: init
	terraform validate

# Show current state
show:
	terraform show

# Import existing resources (usage: make import RESOURCE=honeycombio_trigger.high_error_rate ID=xxx)
import:
	terraform import $(RESOURCE) $(ID)
