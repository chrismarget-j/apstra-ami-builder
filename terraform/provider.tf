terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.45.0"
    }
  }
  backend "s3" {
    encrypt        = true
    bucket         = "t3aco-terraform-state"
    dynamodb_table = "terraform-state-lock"
    key            = "apstra-ami-deployer-go"
    region         = "us-east-1"
  }
}

provider "aws" {
  region = "us-east-1"
}
