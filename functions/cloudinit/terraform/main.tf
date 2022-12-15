locals {
  log_group_arn = "arn:${local.partition}:logs:${local.region}:${local.account}:log-group"
  prefix        = "${var.function_name}-"
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
    ]
  })
}

resource "aws_iam_role_policy_attachment" "aws_lambda_vpc_execution" {
  policy_arn = data.aws_iam_policy.aws_lambda_vpc_execution.arn
  role       = aws_iam_role.ours.id
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
      DEBUG_SKIP_SHUTDOWN = "false"
    }
  }
  vpc_config {
    security_group_ids = [data.aws_security_group.ours.id]
    subnet_ids         = data.aws_subnets.ours.ids
  }
  lifecycle {
    replace_triggered_by = [null_resource.build_project]
  }
}
