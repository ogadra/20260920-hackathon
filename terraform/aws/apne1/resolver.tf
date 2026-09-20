resource "aws_security_group" "resolver_inbound" {
  name_prefix = "bunshin-resolver-in-"
  description = "Route53 Resolver INBOUND endpoint SG"
  vpc_id      = aws_vpc.apne1.id

  tags = merge(local.common_tags, {
    Name    = "bunshin-apne1-resolver-inbound"
    Service = "resolver-inbound"
  })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_security_group_rule" "resolver_inbound_ingress_tcp" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  type              = "ingress"
  from_port         = 53
  to_port           = 53
  protocol          = "tcp"
  cidr_blocks       = [var.google_cloud_dns_forwarder_source_range]
  security_group_id = aws_security_group.resolver_inbound.id
  description       = "DNS TCP from Cloud DNS forwarder source range"
}

resource "aws_security_group_rule" "resolver_inbound_ingress_udp" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  type              = "ingress"
  from_port         = 53
  to_port           = 53
  protocol          = "udp"
  cidr_blocks       = [var.google_cloud_dns_forwarder_source_range]
  security_group_id = aws_security_group.resolver_inbound.id
  description       = "DNS UDP from Cloud DNS forwarder source range"
}

resource "aws_security_group" "resolver_outbound" {
  name_prefix = "bunshin-resolver-out-"
  description = "Route53 Resolver OUTBOUND endpoint SG"
  vpc_id      = aws_vpc.apne1.id

  tags = merge(local.common_tags, {
    Name    = "bunshin-apne1-resolver-outbound"
    Service = "resolver-outbound"
  })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_security_group_rule" "resolver_outbound_egress_tcp" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  type              = "egress"
  from_port         = 53
  to_port           = 53
  protocol          = "tcp"
  cidr_blocks       = var.google_cloud_forwarder_subnet_cidrs
  security_group_id = aws_security_group.resolver_outbound.id
  description       = "DNS TCP to Google Cloud inbound forwarder subnets"
}

resource "aws_security_group_rule" "resolver_outbound_egress_udp" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  type              = "egress"
  from_port         = 53
  to_port           = 53
  protocol          = "udp"
  cidr_blocks       = var.google_cloud_forwarder_subnet_cidrs
  security_group_id = aws_security_group.resolver_outbound.id
  description       = "DNS UDP to Google Cloud inbound forwarder subnets"
}

resource "aws_route53_resolver_endpoint" "inbound" {
  name      = "bunshin-apne1-inbound"
  direction = "INBOUND"

  security_group_ids = [aws_security_group.resolver_inbound.id]

  ip_address {
    subnet_id = aws_subnet.apne1_private[0].id
  }

  ip_address {
    subnet_id = aws_subnet.apne1_private[1].id
  }

  tags = merge(local.common_tags, {
    Name    = "bunshin-apne1-inbound"
    Service = "resolver-inbound"
  })
}

resource "aws_route53_resolver_endpoint" "outbound" {
  name      = "bunshin-apne1-outbound"
  direction = "OUTBOUND"

  security_group_ids = [aws_security_group.resolver_outbound.id]

  ip_address {
    subnet_id = aws_subnet.apne1_private[0].id
  }

  ip_address {
    subnet_id = aws_subnet.apne1_private[1].id
  }

  tags = merge(local.common_tags, {
    Name    = "bunshin-apne1-outbound"
    Service = "resolver-outbound"
  })
}
