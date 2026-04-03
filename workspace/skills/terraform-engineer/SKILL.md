---
name: terraform-engineer
description: "🏗️ Write Terraform configs, design reusable modules, manage remote state, and plan multi-environment deploys. Activate for any IaC task, HCL code, infrastructure provisioning, resource imports, or Terraform plan reviews."
---

# 🏗️ Terraform Engineer

Treat infrastructure as a codebase -- versioned, reviewed, tested, and deployed through pipelines. Every resource should be reproducible from a single `terraform apply`.

## Core Principles

- **Modules are the unit of reuse.** Extract repeated patterns into versioned modules with clear input/output contracts. Pin module versions in consumers.
- **State is sacred.** Store state in remote backends (S3+DynamoDB, GCS, Terraform Cloud). Never commit `terraform.tfstate` to version control. Always enable state locking.
- **Plan before apply, always.** Run `terraform plan -out=tfplan`, review the diff, then `terraform apply tfplan`. Automate this in CI with approval gates.
- **Separate environments by structure, not magic.** Use directory-per-environment or Terragrunt for isolation. Use workspaces only for lightweight variations of the same config.
- **Pin everything.** Pin provider versions, module versions, and the Terraform CLI version. Use `.terraform-version` or `required_version` constraints.

## Workflow

1. **Define the backend.** Configure remote state with locking before writing any resources.
2. **Scaffold the layout.** Choose directory-per-env or workspace-based structure. Create `main.tf`, `variables.tf`, `outputs.tf`, `versions.tf` per module.
3. **Write resources incrementally.** Add a few resources, plan, review, apply. Small changesets are safer than monolithic deploys.
4. **Extract modules.** When a pattern repeats across environments, extract it into `modules/` with typed variables and outputs.
5. **Validate and lint.** Run `terraform validate`, `terraform fmt`, and `tflint` in CI. Use `checkov` or `tfsec` for security scanning.
6. **Plan in CI, apply with approval.** Generate plan artifacts in pull requests. Require human approval before apply in production.

## Examples

### Remote Backend with Locking

```hcl
# versions.tf
terraform {
  required_version = ">= 1.6"

  backend "s3" {
    bucket         = "myco-terraform-state"
    key            = "prod/network/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "terraform-locks"
    encrypt        = true
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}
```

### Directory-per-Environment Layout

```
infra/
  modules/
    vpc/
      main.tf
      variables.tf
      outputs.tf
    rds/
      main.tf
      variables.tf
      outputs.tf
  environments/
    dev/
      main.tf          # calls modules with dev values
      terraform.tfvars
      backend.tf
    staging/
      main.tf
      terraform.tfvars
      backend.tf
    prod/
      main.tf
      terraform.tfvars
      backend.tf
```

### Reusable Module with Typed Variables

```hcl
# modules/vpc/variables.tf
variable "cidr_block" {
  type        = string
  description = "CIDR block for the VPC"
  validation {
    condition     = can(cidrhost(var.cidr_block, 0))
    error_message = "Must be a valid CIDR block."
  }
}

variable "environment" {
  type        = string
  description = "Environment name (dev, staging, prod)"
  validation {
    condition     = contains(["dev", "staging", "prod"], var.environment)
    error_message = "Must be dev, staging, or prod."
  }
}

# modules/vpc/outputs.tf
output "vpc_id" {
  value       = aws_vpc.main.id
  description = "ID of the created VPC"
}
```

## Common Patterns

### Data Sources for External References

```hcl
data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"] # Canonical

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-*-amd64-server-*"]
  }
}

resource "aws_instance" "web" {
  ami           = data.aws_ami.ubuntu.id
  instance_type = var.instance_type
}
```

### Import Existing Infrastructure

```bash
# Step 1: Write the resource block matching the existing resource
# Step 2: Import into state
terraform import aws_s3_bucket.legacy my-existing-bucket

# Step 3: Plan to verify -- the plan should show no changes
terraform plan
```

For Terraform 1.5+, use import blocks declaratively:

```hcl
import {
  to = aws_s3_bucket.legacy
  id = "my-existing-bucket"
}
```

### Explicit Dependencies and Sensitive Values

```hcl
resource "aws_instance" "app" {
  ami           = data.aws_ami.ubuntu.id
  instance_type = "t3.micro"

  # Use depends_on only when Terraform cannot infer the relationship
  depends_on = [aws_iam_role_policy.app]
}

variable "db_password" {
  type      = string
  sensitive = true  # redacted in plan output and logs
}
```

## Anti-Patterns

- **Local state in team projects.** Always use remote backends with locking. Local state causes conflicts and data loss.
- **Hardcoded values.** Extract region, instance type, CIDR blocks, and names into variables. Hardcoded values prevent reuse.
- **Mega-modules.** A module that provisions VPC + RDS + ECS + ALB is too large. Compose small, focused modules instead.
- **`terraform apply` without a saved plan.** The infrastructure may have drifted between plan and apply. Always use `plan -out` and apply the artifact.
- **Using provisioners for configuration management.** Provisioners (remote-exec, local-exec) are a last resort. Prefer user_data, cloud-init, or dedicated tools like Ansible.
- **Workspaces for fundamentally different environments.** Workspaces share the same backend config and code. Use directory-per-env when environments differ in structure, not just variable values.
