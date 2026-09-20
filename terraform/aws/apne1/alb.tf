# trivy:ignore:AVD-AWS-0053 -- target group uses HTTP, HTTPS terminates at ALB
resource "aws_lb_target_group" "internal_nginx" {
  # checkov:skip=CKV_AWS_378:HTTPS terminates at ALB, target uses HTTP
  name        = "bunshin-internal-nginx"
  port        = local.ecs_services["nginx"].port
  protocol    = "HTTP"
  vpc_id      = aws_vpc.apne1.id
  target_type = "ip"

  health_check {
    path                = "/health"
    protocol            = "HTTP"
    matcher             = "200"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  deregistration_delay = 30

  tags = merge(local.common_tags, {
    Service = "nginx"
  })
}

# trivy:ignore:AVD-AWS-0053 -- CloudFront VPC origins reach this ALB over private networking
resource "aws_lb_target_group" "api_ingress_nginx" {
  # checkov:skip=CKV_AWS_378:CloudFront VPC origins reach this ALB over private networking
  name        = "bunshin-api-ingress-nginx"
  port        = local.ecs_services["nginx"].port
  protocol    = "HTTP"
  vpc_id      = aws_vpc.apne1.id
  target_type = "ip"

  health_check {
    path                = "/health"
    protocol            = "HTTP"
    matcher             = "200"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  deregistration_delay = 30

  tags = merge(local.common_tags, {
    Service = "nginx"
  })
}

# trivy:ignore:AVD-AWS-0054 -- ALB access logs are optional for initial deployment
resource "aws_lb" "api_ingress" {
  # checkov:skip=CKV_AWS_91:ALB access logs are optional for initial deployment
  # checkov:skip=CKV2_AWS_20:CloudFront reaches this ALB through VPC origins over private networking
  drop_invalid_header_fields = true
  # checkov:skip=CKV_AWS_150:Deletion protection is not needed for initial deployment
  name     = "bunshin-api-ingress"
  internal = true
  # nginxはX-Forwarded-For末尾からclientのIP:portを読む。
  # Google CloudのX-Bunshin-Edge-Client-Addressと同じ形式に揃える。
  enable_xff_client_port = true
  load_balancer_type     = "application"
  ip_address_type        = "dualstack"
  security_groups = [
    aws_security_group.api_ingress_alb.id,
    aws_security_group.api_ingress_alb_port_forward.id,
  ]
  subnets = aws_subnet.apne1_private[*].id

  tags = merge(local.common_tags, {
    Service = "api-ingress-alb"
  })
}

# trivy:ignore:AVD-AWS-0054 -- ALB access logs are optional for initial deployment
resource "aws_lb" "internal" {
  # checkov:skip=CKV_AWS_91:ALB access logs are optional for initial deployment
  drop_invalid_header_fields = true
  # checkov:skip=CKV_AWS_150:Deletion protection is not needed for initial deployment
  name               = "bunshin-internal"
  internal           = true
  load_balancer_type = "application"
  security_groups    = [aws_security_group.internal_alb.id]
  subnets            = aws_subnet.apne1_private[*].id

  tags = merge(local.common_tags, {
    Service = "internal-alb"
  })
}

resource "aws_lb_listener" "api_ingress_https" {
  load_balancer_arn = aws_lb.api_ingress.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.alb_certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api_ingress_nginx.arn
  }

  tags = merge(local.common_tags, {
    Service = "api-ingress-alb"
  })
}

resource "aws_lb_listener" "api_ingress_https_port_forward" {
  load_balancer_arn = aws_lb.api_ingress.arn
  port              = local.api_ingress_port_forward_port
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.alb_certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api_ingress_nginx.arn
  }

  tags = merge(local.common_tags, {
    Service = "api-ingress-alb"
  })
}

resource "aws_lb_listener" "internal_https" {
  load_balancer_arn = aws_lb.internal.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.alb_certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.internal_nginx.arn
  }

  tags = merge(local.common_tags, {
    Service = "internal-alb"
  })
}
