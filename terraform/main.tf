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

// triggered by eventbridge indications of new snapshots, this function creates Apstra AMIs
module "amimaker" {
  source = "../functions/amimaker/terraform"
  install_ci_lambda_name = module.cloudinit.function_name
  temp_instance_type = "t3a.large"
}

module "cloudinit" {
  source = "../functions/cloudinit/terraform"
}
