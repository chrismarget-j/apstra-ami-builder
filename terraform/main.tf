locals {
  upload_log_group_arn = "arn:${local.partition}:logs:${local.region}:${local.account}:log-group"
  lambda_prefix        = "${var.function_name}-"
  lambda_dir           = "${path.root}/../functions/upload"
  lambda_requirements  = "${local.lambda_dir}/requirements.txt"
  lambda_zip           = "${path.root}/.terraform/${var.function_name}.zip"
}

resource "aws_iam_role" "ours" {
  name_prefix = local.lambda_prefix
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

resource "aws_iam_policy" "ours" {
  name_prefix = local.lambda_prefix
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
          "s3:PutObject",
          #          "s3:GetObject",
          #          "s3:ListBucket"
        ],
        Resource = [
          #          aws_s3_bucket.images.arn,
          "${aws_s3_bucket.ours.arn}/*"
        ]
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "ours" {
  role       = aws_iam_role.ours.name
  policy_arn = aws_iam_policy.ours.arn
}

resource "aws_lambda_function" "ours" {
  function_name = var.function_name
  handler       = basename(local.lambda_bin_file)
  role          = aws_iam_role.ours.arn
  runtime       = "go1.x"
  filename      = data.archive_file.zipped_for_lambda.output_path
  timeout       = 300
  environment {
    variables = {
      BUCKET_NAME = aws_s3_bucket.ours.bucket
    }
  }
  lifecycle {
    replace_triggered_by = [null_resource.build_project]
  }
}
