data "aws_caller_identity" "current" {}

data "aws_acm_certificate" "apne1_alb" {
  provider = aws.apne1

  domain      = var.domain_name
  statuses    = ["ISSUED"]
  most_recent = true
}

data "aws_acm_certificate" "apne3_alb" {
  provider = aws.apne3

  domain      = var.domain_name
  statuses    = ["ISSUED"]
  most_recent = true
}
