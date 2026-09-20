resource "aws_wafv2_web_acl" "api_ingress" {
  # checkov:skip=CKV_AWS_192:Log4j protection is not needed, backend does not use Java
  # checkov:skip=CKV2_AWS_31:WAF logging is not needed for initial deployment
  name  = "bunshin-apne1-api-ingress-waf"
  scope = "REGIONAL"

  default_action {
    allow {}
  }

  rule {
    name     = "aws-common-rule-set"
    priority = 1

    override_action {
      count {}
    }

    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesCommonRuleSet"
        vendor_name = "AWS"
      }
    }

    visibility_config {
      sampled_requests_enabled   = false
      cloudwatch_metrics_enabled = true
      metric_name                = "bunshin-apne1-api-ingress-common-rule-set"
    }
  }

  visibility_config {
    sampled_requests_enabled   = false
    cloudwatch_metrics_enabled = true
    metric_name                = "bunshin-apne1-api-ingress-waf"
  }

  tags = merge(local.common_tags, {
    Name    = "bunshin-apne1-api-ingress-waf"
    Service = "waf"
  })
}

resource "aws_wafv2_web_acl_association" "api_ingress" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  resource_arn = aws_lb.api_ingress.arn
  web_acl_arn  = aws_wafv2_web_acl.api_ingress.arn
}
