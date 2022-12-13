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
      {
        Effect   = "Allow",
        Action   = [
          "ec2:CreateTags",
        ]
        Resource = [
          "aws:${local.partition}:ec2:${local.region}:${local.account}:instance/*"
        ]
      },
      {
        Effect   = "Allow",
        Action   = [
          "ec2:CreateTags",
          "ec2:DescribeImportSnapshotTasks",
          "eC2:DescribeInstances",
          "eC2:RunInstances",
        ]
        Resource = [
          "*",
        ]
      },
      {
        Effect = "Allow",
        Action = [
          "ec2:CreateTags",
          "ec2:RegisterImage",
        ]
        Resource = [
          "arn:${local.partition}:ec2:${local.region}::snapshot/*",
          "arn:${local.partition}:ec2:${local.region}::image/*",
        ]
      },
      {
        Effect = "Allow",
        Action = [
          "ec2:CreateNetworkInterface",
          "ec2:DescribeNetworkInterfaces",
          "ec2:DeleteNetworkInterface",
          "ec2:AssignPrivateIpAddresses",
          "ec2:UnassignPrivateIpAddresses"
        ]
        Resource = "*"
      },
      {
        Effect = "Allow",
        Action = [
          "lambda:InvokeFunction"
        ]
        Resource = "arn:${local.partition}:lambda:${local.region}:${local.account}:function:${var.install_ci_lambda_name}"
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "aws_lambda_vpc_execution" {
  policy_arn = data.aws_iam_policy.aws_lambda_vpc_execution.arn
  role       = aws_iam_role.ours.id
}

resource "aws_security_group" "ours" {
  name_prefix = local.prefix
  description = "Used by temporary Apstra instances during AMI prep, and the lambdas which prep them"
  vpc_id      = data.aws_vpc.ours.id
}

resource "aws_security_group_rule" "local_ssh" {
  security_group_id = aws_security_group.ours.id
  type              = "ingress"
  from_port         = 22
  to_port           = 22
  protocol          = "tcp"
  self              = true
}

resource "aws_security_group_rule" "all_ping" {
  security_group_id = aws_security_group.ours.id
  type              = "ingress"
  from_port         = -1
  to_port           = -1
  protocol          = "icmp"
  cidr_blocks = ["0.0.0.0/0"]
}

resource "aws_security_group_rule" "inbound_from_home" {
  security_group_id = aws_security_group.ours.id
  type              = "ingress"
  from_port         = 22
  to_port           = 22
  protocol          = "tcp"
  cidr_blocks       = ["47.14.45.97/32"]
}

resource "aws_security_group_rule" "outbound" {
  security_group_id = aws_security_group.ours.id
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  self              = true
}

resource "aws_lambda_function" "ours" {
  function_name = var.function_name
  handler       = basename(local.bin_file)
  role          = aws_iam_role.ours.arn
  runtime       = "go1.x"
  filename      = data.archive_file.zipped_for_lambda.output_path
  timeout       = 300
  environment {
    variables = {
      INSTALL_CI_LAMBDA_NAME = var.install_ci_lambda_name
      INSTALL_CI_LAMBDA_SECURITY_GROUP = aws_security_group.ours.id
      INSTANCE_TYPE = var.temp_instance_type
    }
  }
  lifecycle {
    replace_triggered_by = [null_resource.build_project]
  }
}

resource "aws_lambda_permission" "allow_eventbridge" {
  function_name = aws_lambda_function.ours.function_name
  action        = "lambda:InvokeFunction"
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.snapshot.arn
  lifecycle {
    replace_triggered_by = [
      null_resource.build_project,
      aws_lambda_function.ours,
    ]
  }
}