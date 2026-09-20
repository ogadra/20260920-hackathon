resource "aws_route53_zone" "internal" {
  name = "${data.aws_region.current.region}.${var.domain_name}"

  vpc {
    vpc_id     = aws_vpc.apne3.id
    vpc_region = data.aws_region.current.region
  }

  vpc {
    vpc_id     = var.peer_vpc.id
    vpc_region = var.peer_vpc.region
  }

  tags = merge(local.common_tags, {
    Service = "internal-alb"
  })
}

resource "aws_route53_record" "internal_alb" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  zone_id = aws_route53_zone.internal.zone_id
  name    = aws_route53_zone.internal.name
  type    = "A"

  alias {
    name                   = aws_lb.internal.dns_name
    zone_id                = aws_lb.internal.zone_id
    evaluate_target_health = true
  }
}
