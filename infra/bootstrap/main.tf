# Creates the S3 bucket that holds the main stack's Terraform state. This is
# the one piece that can't store its own state remotely (the bucket doesn't
# exist yet), so it keeps a local state file. Run it once per AWS account.

terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.95"
    }
  }
}

variable "region" {
  description = "AWS region for the state bucket."
  type        = string
  default     = "us-east-1"
}

provider "aws" {
  region = var.region
}

data "aws_caller_identity" "current" {}

# Bucket names are global across all of AWS, so the account ID keeps it unique.
resource "aws_s3_bucket" "state" {
  bucket = "raftkv-tfstate-${data.aws_caller_identity.current.account_id}"

  # Losing the state file means Terraform forgets what it created, so make
  # deleting this bucket a deliberate two-step job.
  lifecycle {
    prevent_destroy = true
  }
}

# Every state write keeps the previous version, so a bad apply can be undone.
resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "state" {
  bucket = aws_s3_bucket.state.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# State can contain secrets; it must never be public.
resource "aws_s3_bucket_public_access_block" "state" {
  bucket                  = aws_s3_bucket.state.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

output "state_bucket" {
  value = aws_s3_bucket.state.bucket
}

output "init_command" {
  description = "Run this in infra/aws to point it at the bucket."
  value       = "terraform init -backend-config=\"bucket=${aws_s3_bucket.state.bucket}\" -backend-config=\"region=${var.region}\""
}
