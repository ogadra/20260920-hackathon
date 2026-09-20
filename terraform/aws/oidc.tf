# GitHub OIDC provider for GitHub Actions keyless authentication
resource "aws_iam_openid_connect_provider" "github" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = ["ffffffffffffffffffffffffffffffffffffffff"]

  tags = merge(local.common_tags, {
    Service = "cd"
  })
}

locals {
  # ECS services that need deploy roles with ECR push and ECS deploy permissions
  ecs_deploy_services = toset(["nginx", "broker", "runner"])

  # region set for the ecspresso deploy_ecs policy
  stack_regions = {
    apne1 = "ap-northeast-1"
    apne3 = "ap-northeast-3"
  }
}

# IAM roles for GitHub Actions deployment workflows per service
resource "aws_iam_role" "github_actions_deploy" {
  for_each = local.ecs_deploy_services

  name = "bunshin-deploy-${each.key}"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = aws_iam_openid_connect_provider.github.arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "token.actions.githubusercontent.com:aud" = "sts.amazonaws.com"
          "token.actions.githubusercontent.com:sub" = "repo:ogadra/bunshin:ref:refs/heads/main"
        }
      }
    }]
  })

  tags = merge(local.common_tags, {
    Service = each.key
  })
}

# ECR push permissions scoped to each service repository
resource "aws_iam_role_policy" "deploy_ecr" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  for_each = local.ecs_deploy_services

  name = "bunshin-deploy-${each.key}-ecr"
  role = aws_iam_role.github_actions_deploy[each.key].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ecr:GetDownloadUrlForLayer",
          "ecr:BatchGetImage",
          "ecr:BatchCheckLayerAvailability",
          "ecr:PutImage",
          "ecr:InitiateLayerUpload",
          "ecr:UploadLayerPart",
          "ecr:CompleteLayerUpload",
        ]
        Resource = aws_ecr_repository.service[each.key].arn
      },
      {
        Effect   = "Allow"
        Action   = "ecr:GetAuthorizationToken"
        Resource = "*"
      },
    ]
  })
}

# ECS deploy permissions for ecspresso.
# RegisterTaskDefinition / DescribeTaskDefinition / DeregisterTaskDefinition do
# not support resource-level scoping in IAM, so they are Resource="*"; the
# ecs:TagResource on Resource="*" is required because ecs-task-def.json carries
# tags and RegisterTaskDefinition authorizes TagResource against the not-yet-
# existing task-definition ARN. Service and task mutations are pinned to the
# per-service ARNs; iam:PassRole is scoped to the task/execution roles ecspresso
# registers with new task definitions.
resource "aws_iam_role_policy" "deploy_ecs" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  for_each = local.ecs_deploy_services

  name = "bunshin-deploy-${each.key}-ecs"
  role = aws_iam_role.github_actions_deploy[each.key].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "ServiceOps"
        Effect = "Allow"
        Action = [
          "ecs:DescribeServices",
          "ecs:UpdateService",
          "ecs:DescribeTasks",
          "ecs:ListTasks",
          "ecs:TagResource",
          "ecs:UntagResource",
          "ecs:ListTagsForResource",
        ]
        Resource = concat(
          [
            for region in values(local.stack_regions) :
            "arn:aws:ecs:${region}:${data.aws_caller_identity.current.account_id}:service/bunshin/bunshin-${each.key}"
          ],
          [
            for region in values(local.stack_regions) :
            "arn:aws:ecs:${region}:${data.aws_caller_identity.current.account_id}:task/bunshin/*"
          ],
        )
      },
      {
        Sid    = "TaskDefinitionOps"
        Effect = "Allow"
        Action = [
          "ecs:RegisterTaskDefinition",
          "ecs:DescribeTaskDefinition",
          "ecs:DeregisterTaskDefinition",
          "ecs:TagResource",
        ]
        Resource = "*"
      },
      {
        Sid    = "PassTaskRoles"
        Effect = "Allow"
        Action = "iam:PassRole"
        Resource = flatten([
          for region_slug in keys(local.stack_regions) : [
            "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/bunshin-${region_slug}-${each.key}-task-execution",
            "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/bunshin-${region_slug}-${each.key}-task",
          ]
        ])
        Condition = {
          StringEquals = {
            "iam:PassedToService" = "ecs-tasks.amazonaws.com"
          }
        }
      },
    ]
  })
}
