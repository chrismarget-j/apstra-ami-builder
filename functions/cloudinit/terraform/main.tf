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
#      {
#        Effect   = "Allow",
#        Action   = [
#          "ec2:CreateTags",
#        ]
#        Resource = [
#          "aws:${local.partition}:ec2:${local.region}:${local.account}:instance/*"
#        ]
#      },
#      {
#        Effect   = "Allow",
#        Action   = [
#          "ec2:CreateTags",
#          "ec2:DescribeImportSnapshotTasks",
#          "eC2:DescribeInstances",
#          "eC2:RunInstances",
#        ]
#        Resource = [
#          "*",
#        ]
#      },
#      {
#        Effect = "Allow",
#        Action = [
#          "ec2:CreateTags",
#          "ec2:RegisterImage",
#        ]
#        Resource = [
#          "arn:${local.partition}:ec2:${local.region}::snapshot/*",
#          "arn:${local.partition}:ec2:${local.region}::image/*",
#        ]
#      },
#      {
#        Effect = "Allow",
#        Action = [
#          "ec2:CreateNetworkInterface",
#          "ec2:DescribeNetworkInterfaces",
#          "ec2:DeleteNetworkInterface",
#          "ec2:AssignPrivateIpAddresses",
#          "ec2:UnassignPrivateIpAddresses"
#        ]
#        Resource = "*"
#      }
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
#  environment {
#    variables = {
#      INSTALL_CI_LAMBDA_NAME = var.install_ci_lambda_name
#      INSTALL_CI_LAMBDA_SECURITY_GROUP = aws_security_group.ours.id
#      INSTANCE_TYPE = var.temp_instance_type
#    }
#  }
  lifecycle {
    replace_triggered_by = [null_resource.build_project]
  }
}

#resource "aws_lambda_permission" "allow_ami_builder" {
#  function_name = aws_lambda_function.ours.function_name
#  action        = "lambda:InvokeFunction"
#  principal     = "events.amazonaws.com"
#  source_arn    = aws_cloudwatch_event_rule.snapshot.arn
#  lifecycle {
#    replace_triggered_by = [
#      null_resource.build_project,
#      aws_lambda_function.ours,
#    ]
#  }
#}
