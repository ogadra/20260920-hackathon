data "aws_iam_policy_document" "ecs_tasks_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "ecs_task_execution" {
  for_each = local.ecs_services

  name               = "bunshin-apne3-${each.key}-task-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json

  tags = merge(local.common_tags, {
    Service = each.key
  })
}

data "aws_iam_policy_document" "execution_ecr" {
  for_each = local.ecs_services

  statement {
    effect = "Allow"
    actions = [
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
      "ecr:BatchCheckLayerAvailability",
    ]
    resources = [local.ecr_repository_arns[each.key]]
  }

  statement {
    effect    = "Allow"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "execution_ecr" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  for_each = local.ecs_services

  name   = "bunshin-apne3-${each.key}-execution-ecr"
  role   = aws_iam_role.ecs_task_execution[each.key].id
  policy = data.aws_iam_policy_document.execution_ecr[each.key].json
}

data "aws_iam_policy_document" "execution_logs" {
  for_each = local.ecs_services

  statement {
    effect = "Allow"
    actions = [
      "logs:CreateLogStream",
      "logs:PutLogEvents",
    ]
    resources = ["${aws_cloudwatch_log_group.ecs[each.key].arn}:*"]
  }
}

resource "aws_iam_role_policy" "execution_logs" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  for_each = local.ecs_services

  name   = "bunshin-apne3-${each.key}-execution-logs"
  role   = aws_iam_role.ecs_task_execution[each.key].id
  policy = data.aws_iam_policy_document.execution_logs[each.key].json
}

resource "aws_iam_role" "task" {
  for_each = local.ecs_services

  name               = "bunshin-apne3-${each.key}-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json

  tags = merge(local.common_tags, {
    Service = each.key
  })
}

data "aws_iam_policy_document" "broker_dynamodb" {
  statement {
    effect = "Allow"
    actions = [
      "dynamodb:GetItem",
      "dynamodb:PutItem",
      "dynamodb:UpdateItem",
      "dynamodb:DeleteItem",
    ]
    resources = [
      aws_dynamodb_table.runners_apne3.arn,
    ]
  }

  statement {
    effect = "Allow"
    actions = [
      "dynamodb:Query",
    ]
    resources = [
      "${aws_dynamodb_table.runners_apne3.arn}/index/session-index",
      "${aws_dynamodb_table.runners_apne3.arn}/index/state-index",
    ]
  }
}

resource "aws_iam_role_policy" "broker_dynamodb" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  name   = "bunshin-apne3-broker-dynamodb"
  role   = aws_iam_role.task["broker"].id
  policy = data.aws_iam_policy_document.broker_dynamodb.json
}
