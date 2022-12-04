module "upload" {
  source = "../functions/upload/terraform"
  bucket_name = aws_s3_bucket.ours.bucket
}
