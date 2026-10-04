# Reference module — provider is configured by the caller (root), per module best
# practice. No provider block here on purpose.
terraform {
  required_version = ">= 1.5"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.40"
    }
  }
}
