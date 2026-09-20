data "aws_iam_policy_document" "flow_logs_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["vpc-flow-logs.amazonaws.com"]
    }

    # Bind the service principal to this account's flow logs so a flow log in
    # another account cannot assume this role (confused-deputy guard).
    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }

    condition {
      test     = "ArnLike"
      variable = "aws:SourceArn"
      values   = ["arn:aws:ec2:*:${data.aws_caller_identity.current.account_id}:vpc-flow-log/*"]
    }
  }
}

resource "aws_iam_role" "flow_logs" {
  name               = "bunshin-vpc-flow-logs"
  assume_role_policy = data.aws_iam_policy_document.flow_logs_assume_role.json

  tags = local.common_tags
}

data "aws_iam_policy_document" "flow_logs" {
  statement {
    effect = "Allow"
    actions = [
      "logs:CreateLogStream",
      "logs:PutLogEvents",
      "logs:DescribeLogStreams",
    ]
    resources = [
      "${aws_cloudwatch_log_group.flow_logs_apne1.arn}:*",
      "${aws_cloudwatch_log_group.flow_logs_apne3.arn}:*",
    ]
  }

  statement {
    effect = "Allow"
    actions = [
      "logs:CreateLogGroup",
    ]
    resources = [
      aws_cloudwatch_log_group.flow_logs_apne1.arn,
      aws_cloudwatch_log_group.flow_logs_apne3.arn,
    ]
  }

  statement {
    effect = "Allow"
    actions = [
      "logs:DescribeLogGroups",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "flow_logs" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  name   = "bunshin-vpc-flow-logs"
  role   = aws_iam_role.flow_logs.id
  policy = data.aws_iam_policy_document.flow_logs.json
}

# trivy:ignore:AVD-AWS-0017 -- KMS encryption is not required for this use case
resource "aws_cloudwatch_log_group" "flow_logs_apne1" {
  # checkov:skip=CKV_AWS_158:KMS encryption is not required for this use case
  provider = aws.apne1

  name                        = "/vpc/bunshin-flow-logs"
  retention_in_days           = 365
  deletion_protection_enabled = true

  tags = local.common_tags
}

resource "aws_flow_log" "apne1" {
  provider = aws.apne1

  iam_role_arn    = aws_iam_role.flow_logs.arn
  log_destination = aws_cloudwatch_log_group.flow_logs_apne1.arn
  traffic_type    = "ALL"
  vpc_id          = module.apne1.vpc_id

  tags = merge(local.common_tags, {
    Name = "bunshin-apne1"
  })
}

# trivy:ignore:AVD-AWS-0017 -- KMS encryption is not required for this use case
resource "aws_cloudwatch_log_group" "flow_logs_apne3" {
  # checkov:skip=CKV_AWS_158:KMS encryption is not required for this use case
  provider = aws.apne3

  name                        = "/vpc/bunshin-flow-logs"
  retention_in_days           = 365
  deletion_protection_enabled = true

  tags = local.common_tags
}

resource "aws_flow_log" "apne3" {
  provider = aws.apne3

  iam_role_arn    = aws_iam_role.flow_logs.arn
  log_destination = aws_cloudwatch_log_group.flow_logs_apne3.arn
  traffic_type    = "ALL"
  vpc_id          = module.apne3.vpc_id

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3"
  })
}
