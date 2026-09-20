resource "aws_vpc_peering_connection" "apne1_apne3" {
  provider = aws.apne1

  vpc_id      = module.apne1.vpc_id
  peer_vpc_id = module.apne3.vpc_id
  peer_region = "ap-northeast-3"
  auto_accept = false

  tags = merge(local.common_tags, {
    Name = "bunshin-apne1-apne3"
  })
}

resource "aws_vpc_peering_connection_accepter" "apne1_apne3" {
  provider = aws.apne3

  vpc_peering_connection_id = aws_vpc_peering_connection.apne1_apne3.id
  auto_accept               = true

  tags = merge(local.common_tags, {
    Name = "bunshin-apne1-apne3"
  })
}

resource "aws_vpc_peering_connection_options" "apne1_apne3_requester" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne1

  vpc_peering_connection_id = aws_vpc_peering_connection_accepter.apne1_apne3.id

  requester {
    allow_remote_vpc_dns_resolution = true
  }
}

resource "aws_vpc_peering_connection_options" "apne1_apne3_accepter" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne3

  vpc_peering_connection_id = aws_vpc_peering_connection_accepter.apne1_apne3.id

  accepter {
    allow_remote_vpc_dns_resolution = true
  }
}

resource "aws_security_group_rule" "apne1_nginx_egress_apne3_internal_alb" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne1

  type              = "egress"
  from_port         = 443
  to_port           = 443
  protocol          = "tcp"
  cidr_blocks       = module.apne3.private_subnet_cidrs
  security_group_id = module.apne1.nginx_security_group_id
  description       = "HTTPS to apne3 internal ALB"
}

resource "aws_security_group_rule" "apne3_internal_alb_ingress_apne1_nginx" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne3

  type              = "ingress"
  from_port         = 443
  to_port           = 443
  protocol          = "tcp"
  cidr_blocks       = module.apne1.private_subnet_cidrs
  security_group_id = module.apne3.internal_alb_security_group_id
  description       = "HTTPS from apne1 nginx"
}

resource "aws_security_group_rule" "apne3_nginx_egress_apne1_internal_alb" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne3

  type              = "egress"
  from_port         = 443
  to_port           = 443
  protocol          = "tcp"
  cidr_blocks       = module.apne1.private_subnet_cidrs
  security_group_id = module.apne3.nginx_security_group_id
  description       = "HTTPS to apne1 internal ALB"
}

resource "aws_security_group_rule" "apne1_internal_alb_ingress_apne3_nginx" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne1

  type              = "ingress"
  from_port         = 443
  to_port           = 443
  protocol          = "tcp"
  cidr_blocks       = module.apne3.private_subnet_cidrs
  security_group_id = module.apne1.internal_alb_security_group_id
  description       = "HTTPS from apne3 nginx"
}
