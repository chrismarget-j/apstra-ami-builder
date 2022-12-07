locals {
  upload_log_group_arn = "arn:${local.partition}:logs:${local.region}:${local.account}:log-group"
  prefix               = "${var.function_name}-"
}

resource "aws_iam_role" "ours" {
  name_prefix = local.prefix
  assume_role_policy = jsonencode({
    Version = "2012-10-17",
    Statement = [
      {
        Effect = "Allow",
        Action = "sts:AssumeRole"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy" "ours" {
  name_prefix = local.prefix
  role        = aws_iam_role.ours.id
  policy = jsonencode({
    Version = "2012-10-17",
    Statement = [
      {
        Effect = "Allow",
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ],
        Resource = [
          "${local.upload_log_group_arn}:/aws/lambda/${aws_lambda_function.ours.function_name}:*",
          "${local.upload_log_group_arn}:/aws/lambda/${aws_lambda_function.ours.function_name}:log-stream:*"
        ]
      },
      {
        Effect = "Allow",
        Action = [
          "s3:GetBucketLocation",
          "s3:GetObject",
          "s3:GetObjectTagging",
        ]
        Resource = [
          data.aws_s3_bucket.vmdk.arn,
          "${data.aws_s3_bucket.vmdk.arn}/*",
          "${data.aws_s3_bucket.vmdk.arn}/${local.bucket_key_prefix}*${local.bucket_key_suffix}/*"
        ]
      },
      {
        Effect = "Allow",
        Action = [
          "ec2:ImportSnapshot",
          "ec2:CopySnapshot",
          "ec2:CreateTags",
        ]
        Resource = [
#          "arn:${data.aws_partition.ours.id}:ec2:${data.aws_region.ours.id}:${data.aws_caller_identity.ours.account_id}:snapshot/*",
#          "arn:${data.aws_partition.ours.id}:ec2:${data.aws_region.ours.id}:${data.aws_caller_identity.ours.account_id}:import-snapshot-task/*",
          "*",
          "arn:aws:ec2:us-east-1:086704128018:import-snapshot-task/*",
          "arn:aws:ec2:us-east-1::import-snapshot-task/*",
          #          "arn:${data.aws_partition.ours.id}:ec2:${data.aws_region.ours.id}::snapshot/*",
          #          "arn:${data.aws_partition.ours.id}:ec2:${data.aws_region.ours.id}::import-snapshot-task/*",
          #          "*"
        ]
      },
    ]
  })
}

resource "aws_iam_role" "vmimport" {
  name = "vmimport"
  assume_role_policy = jsonencode({
    Version = "2012-10-17",
    Statement = [
      {
        Effect    = "Allow",
        Principal = { "Service" : "vmie.amazonaws.com" },
        Action    = "sts:AssumeRole",
        Condition = {
          StringEquals = {
            "sts:Externalid" : "vmimport"
          }
        }
      }
    ]
  })
}

resource "aws_iam_role_policy" "vmimport" {
  role = aws_iam_role.vmimport.id
  policy = jsonencode({
    Version = "2012-10-17",
    Statement = [
      {
        Effect = "Allow",
        Action = [
          "s3:GetBucketLocation",
          "s3:GetObject",
          "s3:ListBucket"
        ],
        Resource = [
          data.aws_s3_bucket.vmdk.arn,
          "${data.aws_s3_bucket.vmdk.arn}/*",
        ]
      },
      {
        Effect = "Allow",
        Action = [
          "s3:GetBucketLocation",
          "s3:GetObject",
          "s3:ListBucket",
          "s3:PutObject",
          "s3:GetBucketAcl"
        ],
        Resource = [
          "arn:aws:s3:::export-bucket",
          "arn:aws:s3:::export-bucket/*"
        ]
      },
      {
        Effect = "Allow",
        Action = [
          "ec2:ModifySnapshotAttribute",
          "ec2:CopySnapshot",
          "ec2:RegisterImage",
          "ec2:Describe*"
        ],
        Resource = "*"
      }
    ]
  })
}

resource "aws_lambda_function" "ours" {
  function_name = var.function_name
  handler       = basename(local.bin_file)
  role          = aws_iam_role.ours.arn
  runtime       = "go1.x"
  filename      = data.archive_file.zipped_for_lambda.output_path
  timeout       = 180
  environment {
    variables = {
      ROLE_NAME = aws_iam_role.vmimport.name
    }
  }
  lifecycle {
    replace_triggered_by = [null_resource.build_project]
  }
}
