variable "function_name" {
  description = "name to use when deploying this function on AWS Lambda"
  type    = string
  default = "apstra-ami-prep"
}

variable "install_ci_lambda_name" {
  description = "name of lambda which sets up cloud-init on a temporary Apstra instance"

}

variable "temp_instance_type" {
  description = "instance type to use while prepping Apstra AMI"

}
