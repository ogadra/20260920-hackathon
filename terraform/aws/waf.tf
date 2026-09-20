resource "aws_wafv2_web_acl" "cloudfront" {
  # checkov:skip=CKV_AWS_192:Log4j protection is not needed, backend does not use Java
  # checkov:skip=CKV2_AWS_31:WAF logging is not needed for initial deployment
  provider = aws.use1
  name     = "bunshin-cloudfront-waf"
  scope    = "CLOUDFRONT"

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
      metric_name                = "bunshin-cloudfront-common-rule-set"
    }
  }

  visibility_config {
    sampled_requests_enabled   = false
    cloudwatch_metrics_enabled = true
    metric_name                = "bunshin-cloudfront-waf"
  }

  tags = merge(local.common_tags, {
    Name    = "bunshin-cloudfront-waf"
    Service = "waf"
  })
}
