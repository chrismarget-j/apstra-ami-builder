resource "aws_s3_bucket_notification" "bucket_notification" {
  bucket = var.vmdk_bucket_name

  lambda_function {
    lambda_function_arn = aws_lambda_function.ours.arn
    events              = ["s3:ObjectCreated:Put"]
    filter_prefix       = local.bucket_key_prefix
    filter_suffix       = local.bucket_key_suffix
  }

  depends_on = [
    aws_lambda_permission.allow_bucket,
    aws_lambda_function.ours
  ]
}

resource "aws_lambda_permission" "allow_bucket" {
  statement_id  = "AllowExecutionFromS3Bucket"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.ours.function_name
  principal     = "s3.amazonaws.com"
  source_arn    = "arn:aws:s3:::${var.vmdk_bucket_name}"
  lifecycle {
    replace_triggered_by = [
      null_resource.build_project,
      aws_lambda_function.ours,
    ]
  }
}
