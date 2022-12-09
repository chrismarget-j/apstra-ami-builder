// grab Apstra .ova from Juniper, extract .vmdk, stash in S3
module "upload" {
  source = "../functions/upload/terraform"
  vmdk_bucket_name = aws_s3_bucket.ours.bucket
}

// convert Apstra .vmdk from S3 blob to EC2 EBS snapshot
module "snapshot" {
  source = "../functions/snapshot/terraform"
  vmdk_bucket_name = module.upload.vmdk_bucket_name
}

//
module "installci" {
  source = "../functions/installci/terraform"
}