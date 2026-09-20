resource "aws_secretsmanager_secret" "jev_api_key" {
  # checkov:skip=CKV_AWS_149:AWS managed encryption is sufficient for this use case
  # checkov:skip=CKV2_AWS_57:The Jev API key is issued by TypeSafe AI, so AWS cannot rotate it
  name        = "bunshin-jev-api-key"
  description = "Jev API key used by the runner to validate commands"

  recovery_window_in_days = 0

  tags = merge(local.common_tags, {
    Name    = "bunshin-jev-api-key"
    Service = "runner"
  })
}
