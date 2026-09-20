resource "aws_security_group_rule" "apne1_nginx_egress_google_cloud_internal_lb" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne1

  type              = "egress"
  from_port         = 443
  to_port           = 443
  protocol          = "tcp"
  cidr_blocks       = local.google_cloud_internal_lb_cidrs
  security_group_id = data.aws_security_group.apne1_nginx.id
  description       = "HTTPS to Google Cloud internal LB across HA VPN"
}

resource "aws_security_group_rule" "apne3_nginx_egress_google_cloud_internal_lb" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne3

  type              = "egress"
  from_port         = 443
  to_port           = 443
  protocol          = "tcp"
  cidr_blocks       = local.google_cloud_internal_lb_cidrs
  security_group_id = data.aws_security_group.apne3_nginx.id
  description       = "HTTPS to Google Cloud internal LB across HA VPN"
}

# GKE Autopilotはip-masq-agentのdefaultでRFC1918宛のPod source IPを保持する
resource "aws_security_group_rule" "apne1_internal_alb_ingress_google_cloud_nginx" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne1

  type              = "ingress"
  from_port         = 443
  to_port           = 443
  protocol          = "tcp"
  cidr_blocks       = local.google_cloud_pod_secondary_cidrs
  security_group_id = data.aws_security_group.apne1_internal_alb.id
  description       = "HTTPS from Google Cloud nginx Pod across HA VPN"
}

resource "aws_security_group_rule" "apne3_internal_alb_ingress_google_cloud_nginx" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne3

  type              = "ingress"
  from_port         = 443
  to_port           = 443
  protocol          = "tcp"
  cidr_blocks       = local.google_cloud_pod_secondary_cidrs
  security_group_id = data.aws_security_group.apne3_internal_alb.id
  description       = "HTTPS from Google Cloud nginx Pod across HA VPN"
}
