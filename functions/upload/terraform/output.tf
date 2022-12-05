output "vmdk_bucket_name" {
  value = var.vmdk_bucket_name
}

output "function_name" {
  value = aws_lambda_function.ours.function_name
}