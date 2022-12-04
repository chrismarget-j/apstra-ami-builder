module "upload" {
  source = "../functions/upload/terraform"
  vmdk_bucket_name = aws_s3_bucket.ours.bucket
}
