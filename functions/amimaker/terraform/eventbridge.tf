// eventbridge rule to trigger our labmda whenever a snapshot is successfully
// copied (imports from S3 seem to involve an import to somewhere else and then
// a copy into our account)
resource "aws_cloudwatch_event_rule" "snapshot" {
  name_prefix = local.prefix
  description = "Detect creation of new EBS snapshots"

  event_pattern = jsonencode({
    source = ["aws.ec2"],
    detail-type = ["EBS Snapshot Notification"]
    detail = {
      event = ["copySnapshot"]
      result = ["succeeded"]
    }
  })
}

resource "aws_cloudwatch_event_target" "snapshot" {
  arn  = aws_lambda_function.ours.arn
  rule = aws_cloudwatch_event_rule.snapshot.name
}
