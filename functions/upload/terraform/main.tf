locals {
  log_group_arn = "arn:${local.partition}:logs:${local.region}:${local.account}:log-group"
  prefix        = "${var.function_name}-"
}

resource "aws_iam_role" "vmimport" {
  name_prefix = "${local.prefix}vmimport-"
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
  role        = aws_iam_role.ours.id
  name_prefix = local.prefix
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
          "${local.log_group_arn}:/aws/lambda/${aws_lambda_function.ours.function_name}:*",
          "${local.log_group_arn}:/aws/lambda/${aws_lambda_function.ours.function_name}:log-stream:*"
        ]
      },
      {
        Effect = "Allow",
        Action = [
          "s3:PutObject",
          "s3:GetObject",
          "s3:PutObjectTagging",
          "s3:GetObjectTagging",
        ]
        Resource = [
          "${data.aws_s3_bucket.vmdk.arn}/*",
          data.aws_s3_bucket.vmdk.arn
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "ec2:CreateTags",
        ]
        Resource = "arn:${data.aws_partition.ours.id}:ec2:${data.aws_region.ours.id}:${data.aws_caller_identity.ours.account_id}:import-snapshot-task/*"
      },
      {
        Effect = "Allow"
        Action = "ec2:ImportSnapshot"
        Resource = [
          "arn:${data.aws_partition.ours.id}:ec2:${data.aws_region.ours.id}::snapshot/*",
          "arn:${data.aws_partition.ours.id}:ec2:${data.aws_region.ours.id}:${data.aws_caller_identity.ours.account_id}:import-snapshot-task/*",
        ]
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
      BUCKET_NAME = var.vmdk_bucket_name
      VM_IMPORT_ROLE_NAME = aws_iam_role.vmimport.name
    }
  }
  lifecycle {
    replace_triggered_by = [null_resource.build_project]
  }
}
