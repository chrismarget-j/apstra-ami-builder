locals {
  upload_log_group_arn = "arn:${local.partition}:logs:${local.region}:${local.account}:log-group"
  prefix               = "${var.function_name}-"
  lambda_zip           = "${local.bin_file}.zip"
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

resource "aws_iam_policy" "ours" {
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
          "${local.upload_log_group_arn}:/aws/lambda/${aws_lambda_function.ours.function_name}:*",
          "${local.upload_log_group_arn}:/aws/lambda/${aws_lambda_function.ours.function_name}:log-stream:*"
        ]
      },
      {
        Effect   = "Allow",
        Action   = "s3:PutObject",
        Resource = "${data.aws_s3_bucket.ours.arn}/*"
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
  handler       = basename(local.bin_file)
  role          = aws_iam_role.ours.arn
  runtime       = "go1.x"
  filename      = data.archive_file.zipped_for_lambda.output_path
  timeout       = 180
  environment {
    variables = {
      BUCKET_NAME = var.bucket_name
    }
  }
  lifecycle {
    replace_triggered_by = [null_resource.build_project]
  }
}
