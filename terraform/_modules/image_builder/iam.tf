resource "aws_iam_instance_profile" "image_builder" {
  name = aws_iam_role.image_builder.name
  role = aws_iam_role.image_builder.name
}

resource "aws_iam_role" "image_builder" {
  name_prefix = "${local.project_name}-image-builder-"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      },
    ]
  })
}

resource "aws_iam_role_policy_attachment" "image_builder_EC2InstanceProfileForImageBuilder" {
  role       = aws_iam_role.image_builder.name
  policy_arn = "arn:aws:iam::aws:policy/EC2InstanceProfileForImageBuilder"
}

resource "aws_iam_role_policy_attachment" "image_builder_AmazonSSMManagedInstanceCore" {
  role       = aws_iam_role.image_builder.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_policy" "image_builder" {
  name_prefix = "${local.project_name}-image_builder-"
  policy = jsonencode({
    Version = "2012-10-17",
    Statement = [
      {
        Effect   = "Allow",
        Action   = "s3:GetObject",
        Resource = "${aws_s3_bucket.ours.arn}/*"
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "image_builder" {
  role       = aws_iam_role.image_builder.name
  policy_arn = aws_iam_policy.image_builder.arn
}

resource "aws_iam_instance_profile" "apstra_ami_builder" {
  name = aws_iam_role.apstra_ami_builder.name
  role = aws_iam_role.apstra_ami_builder.name
}

resource "aws_iam_role" "apstra_ami_builder" {
  name_prefix = "${local.project_name}-apstra_ami_builder-"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      },
    ]
  })
}

resource "aws_iam_policy" "apstra_ami_builder" {
  name_prefix = "${local.project_name}-apstra_ami_builder-"
  policy = jsonencode({
    Version = "2012-10-17",
    Statement = [
      {
        Effect   = "Allow",
        Action   = [
          "ec2:CreateVolume",
          "ec2:DeleteVolume",
        ]
        Resource = "arn:aws:ec2:*:${data.aws_caller_identity.ours.account_id}:volume/*"
      },
      {
        Effect = "Allow",
        Action = [
          "ec2:DescribeInstances",
          "ec2:DescribeVolumes",
          "ec2:DescribeSnapshots"
        ],
        Resource = "*"
      },
      {
        Effect = "Allow",
        Action = [
          "ec2:DetachVolume",
          "ec2:AttachVolume"
        ],
        Resource = [
          "arn:aws:ec2:*:${data.aws_caller_identity.ours.account_id}:instance/*",
          "arn:aws:ec2:*:${data.aws_caller_identity.ours.account_id}:volume/*"
        ]
      },
      {
        Effect = "Allow",
        Action = [
          "ec2:CreateSnapshot",
          "ec2:CreateTags",
        ]
        Resource = [
          "arn:aws:ec2:*::snapshot/*",
          "arn:aws:ec2:*:${data.aws_caller_identity.ours.account_id}:volume/*",
          "arn:aws:ec2:*::image/*",
        ]
      },
      {
        Effect = "Allow",
        Action = "ec2:RegisterImage",
        Resource = [
          "arn:aws:ec2:*::snapshot/*",
          "arn:aws:ec2:*::image/*"
        ]
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "apstra_ami_builder" {
  role       = aws_iam_role.apstra_ami_builder.name
  policy_arn = aws_iam_policy.apstra_ami_builder.arn
}

resource "aws_iam_role_policy_attachment" "apstra_ami_builder_AmazonSSMManagedInstanceCore" {
  role       = aws_iam_role.apstra_ami_builder.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}
