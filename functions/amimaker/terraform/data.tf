data "aws_region" "ours" {}

data "aws_caller_identity" "ours" {}

data "aws_partition" "ours" {}

locals {
  partition            = data.aws_partition.ours.partition
  region               = data.aws_region.ours.name
  account              = data.aws_caller_identity.ours.account_id
}

data "aws_vpc" "ours" {
  default = true
}

data "aws_security_group" "default" {
  name = "default"
}

data "aws_iam_policy" "aws_lambda_vpc_execution" {
  name = "AWSLambdaVPCAccessExecutionRole"
}

data "external" "current_ip" {
  program = ["bash", "-c", "jq -n --arg ip $(curl -s https://checkip.amazonaws.com) '{\"ip\":$ip}'"]
}
