terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.95"
    }
  }

  # State lives in the bucket made by infra/bootstrap. The bucket name is
  # account-specific, so it's passed at init time (see the bootstrap output)
  # rather than hard-coded. use_lockfile stops two applies running at once.
  backend "s3" {
    key          = "raftkv/aws.tfstate"
    encrypt      = true
    use_lockfile = true
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project   = "raftkv"
      ManagedBy = "terraform"
    }
  }
}
