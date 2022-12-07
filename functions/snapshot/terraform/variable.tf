variable "function_name" {
  description = "name to use when deploying this function on AWS Lambda"
  type    = string
  default = "vmdk-to-ami"
}

variable "vmdk_bucket_name" {
  description = "name of S3 bucket where new VMDK files are dropped"
  type = string
}
