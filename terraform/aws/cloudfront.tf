data "aws_cloudfront_cache_policy" "caching_disabled" {
  name = "Managed-CachingDisabled"
}

# port-forwardの32 hex labelを含むHostをそのままorigin (= nginx)へ渡すため、
# CloudFront側で書き換えないManaged-AllViewerを使う。
data "aws_cloudfront_origin_request_policy" "all_viewer" {
  name = "Managed-AllViewer"
}

# port-forwardのdistributionはstackごとに1本ずつ持ち、Hostをそのまま
# origin (nginx)へ通す。origin portはstackごとに固定 (apne1=8443, apne3=9443)。
# trivy:ignore:AVD-AWS-0010 -- CloudFront access logs are not required for the initial deployment
resource "aws_cloudfront_distribution" "port_forward_apne1" {
  # checkov:skip=CKV_AWS_86:CloudFront access logs are not required for the initial deployment
  # checkov:skip=CKV_AWS_305:port-forward serves dynamic runner apps, no default root object
  # checkov:skip=CKV_AWS_310:Global Accelerator handles health-aware API origin routing
  # checkov:skip=CKV_AWS_374:Geo restriction is not required for this service
  # checkov:skip=CKV2_AWS_32:Response headers are owned by the runner app; CloudFront must not overwrite them
  # checkov:skip=CKV2_AWS_47:Log4j protection is not needed, backend does not use Java
  enabled         = true
  is_ipv6_enabled = true
  comment         = "Bunshin port-forward apne1"
  aliases         = ["*.ap-northeast-1.${var.domain_name}"]
  price_class     = "PriceClass_200"
  web_acl_id      = aws_wafv2_web_acl.cloudfront.arn

  origin {
    domain_name = local.api_ingress_origin_domain_name
    origin_id   = "api-ingress-apne1-pf"

    custom_origin_config {
      http_port              = 80
      https_port             = 8443
      origin_protocol_policy = "https-only"
      origin_ssl_protocols   = ["TLSv1.2"]
    }
  }

  default_cache_behavior {
    allowed_methods          = ["DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"]
    cached_methods           = ["GET", "HEAD", "OPTIONS"]
    cache_policy_id          = data.aws_cloudfront_cache_policy.caching_disabled.id
    origin_request_policy_id = data.aws_cloudfront_origin_request_policy.all_viewer.id
    compress                 = false
    target_origin_id         = "api-ingress-apne1-pf"
    viewer_protocol_policy   = "redirect-to-https"
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    acm_certificate_arn      = aws_acm_certificate_validation.cloudfront_port_forward_apne1.certificate_arn
    minimum_protocol_version = "TLSv1.2_2021"
    ssl_support_method       = "sni-only"
  }

  tags = merge(local.common_tags, {
    Name    = "bunshin-cloudfront-pf-apne1"
    Service = "cloudfront"
  })
}

# trivy:ignore:AVD-AWS-0010 -- CloudFront access logs are not required for the initial deployment
resource "aws_cloudfront_distribution" "port_forward_apne3" {
  # checkov:skip=CKV_AWS_86:CloudFront access logs are not required for the initial deployment
  # checkov:skip=CKV_AWS_305:port-forward serves dynamic runner apps, no default root object
  # checkov:skip=CKV_AWS_310:Global Accelerator handles health-aware API origin routing
  # checkov:skip=CKV_AWS_374:Geo restriction is not required for this service
  # checkov:skip=CKV2_AWS_32:Response headers are owned by the runner app; CloudFront must not overwrite them
  # checkov:skip=CKV2_AWS_47:Log4j protection is not needed, backend does not use Java
  enabled         = true
  is_ipv6_enabled = true
  comment         = "Bunshin port-forward apne3"
  aliases         = ["*.ap-northeast-3.${var.domain_name}"]
  price_class     = "PriceClass_200"
  web_acl_id      = aws_wafv2_web_acl.cloudfront.arn

  origin {
    domain_name = local.api_ingress_origin_domain_name
    origin_id   = "api-ingress-apne3-pf"

    custom_origin_config {
      http_port              = 80
      https_port             = 9443
      origin_protocol_policy = "https-only"
      origin_ssl_protocols   = ["TLSv1.2"]
    }
  }

  default_cache_behavior {
    allowed_methods          = ["DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"]
    cached_methods           = ["GET", "HEAD", "OPTIONS"]
    cache_policy_id          = data.aws_cloudfront_cache_policy.caching_disabled.id
    origin_request_policy_id = data.aws_cloudfront_origin_request_policy.all_viewer.id
    compress                 = false
    target_origin_id         = "api-ingress-apne3-pf"
    viewer_protocol_policy   = "redirect-to-https"
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    acm_certificate_arn      = aws_acm_certificate_validation.cloudfront_port_forward_apne3.certificate_arn
    minimum_protocol_version = "TLSv1.2_2021"
    ssl_support_method       = "sni-only"
  }

  tags = merge(local.common_tags, {
    Name    = "bunshin-cloudfront-pf-apne3"
    Service = "cloudfront"
  })
}
