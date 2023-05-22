data "aws_partition" "ours" {}
data "aws_region" "ours" {}
data "aws_caller_identity" "ours" {}

data "aws_ami" "ubuntu_2204" {
  owners = ["099720109477"] # Canonical
  most_recent = true
  filter {
    name   = "name"
#    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"]
    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-20230115"]
  }

}
