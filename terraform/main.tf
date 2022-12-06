module "upload" {
  source = "../functions/upload/terraform"
  vmdk_bucket_name = aws_s3_bucket.ours.bucket
}

module "toami" {
  source = "../functions/toami/terraform"
  vmdk_bucket_name = module.upload.vmdk_bucket_name
}
