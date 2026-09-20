resource "aws_acm_certificate" "cloudfront_port_forward_apne1" {
  provider = aws.use1

  domain_name       = "*.ap-northeast-1.${var.domain_name}"
  validation_method = "DNS"

  tags = merge(local.common_tags, {
    Name    = "bunshin-cloudfront-pf-apne1"
    Service = "cloudfront"
  })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_acm_certificate" "cloudfront_port_forward_apne3" {
  provider = aws.use1

  domain_name       = "*.ap-northeast-3.${var.domain_name}"
  validation_method = "DNS"

  tags = merge(local.common_tags, {
    Name    = "bunshin-cloudfront-pf-apne3"
    Service = "cloudfront"
  })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_acm_certificate_validation" "cloudfront_port_forward_apne1" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.use1

  certificate_arn = aws_acm_certificate.cloudfront_port_forward_apne1.arn
  validation_record_fqdns = [
    for opt in aws_acm_certificate.cloudfront_port_forward_apne1.domain_validation_options : opt.resource_record_name
  ]

  timeouts {
    create = "1m"
  }
}

resource "aws_acm_certificate_validation" "cloudfront_port_forward_apne3" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.use1

  certificate_arn = aws_acm_certificate.cloudfront_port_forward_apne3.arn
  validation_record_fqdns = [
    for opt in aws_acm_certificate.cloudfront_port_forward_apne3.domain_validation_options : opt.resource_record_name
  ]

  timeouts {
    create = "1m"
  }
}
