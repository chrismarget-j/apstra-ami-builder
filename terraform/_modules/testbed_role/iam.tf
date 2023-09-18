resource "aws_iam_user" "apstra_testbed" {
  name = "terraform_integration_test_runner"
}

resource "aws_iam_policy" "apstra_testbed" {
  name = "terraform_integration_test_runner"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["dynamodb:DeleteItem", "dynamodb:GetItem", "dynamodb:PutItem"]
        Resource = "arn:aws:dynamodb:us-east-1:086704128018:table/terraform-state-lock"
      },
      {
        Effect   = "Allow"
        Action   = ["s3:GetObject", "s3:PutObject"]
        Resource = "arn:aws:s3:::t3aco-terraform-state/apstra-testbed",
      }
    ]
  })
}

resource "aws_iam_user_policy_attachment" "apstra_testbed" {
  policy_arn = aws_iam_policy.apstra_testbed.arn
  user       = aws_iam_user.apstra_testbed.name
}

output "foo" {
  value = aws_iam_policy.apstra_testbed
}
