resource "aws_s3_bucket" "ours" {
  bucket_prefix = "${local.project_name}-"
  force_destroy = true
}

resource "aws_s3_bucket_public_access_block" "ours" {
  bucket                  = aws_s3_bucket.ours.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_object" "ami_builder" {
  bucket = aws_s3_bucket.ours.bucket
  key    = "ami_builder.sh"
  source = "${path.module}/ami_builder.sh"
  etag = filemd5("${path.module}/ami_builder.sh")
}