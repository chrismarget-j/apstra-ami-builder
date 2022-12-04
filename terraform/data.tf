data "aws_region" "ours" {}

data "aws_caller_identity" "ours" {}

data "aws_partition" "ours" {}

locals {
  partition            = data.aws_partition.ours.partition
  region               = data.aws_region.ours.name
  account              = data.aws_caller_identity.ours.account_id
}
