resource "aws_vpc_endpoint" "apne3_dynamodb" {
  vpc_id       = aws_vpc.apne3.id
  service_name = "com.amazonaws.ap-northeast-3.dynamodb"

  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.apne3_private.id]

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3-dynamodb"
  })
}

resource "aws_security_group" "apne3_vpc_endpoint_for_ecs" {
  name_prefix = "bunshin-vpc-ep-ecs-"
  description = "Security group for VPC endpoints used by ECS tasks"
  vpc_id      = aws_vpc.apne3.id

  tags = merge(local.common_tags, {
    Name    = "bunshin-apne3-vpc-endpoint-for-ecs"
    Service = "vpc-endpoint"
  })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_security_group_rule" "apne3_vpc_endpoint_for_ecs_ingress" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  for_each = {
    nginx  = aws_security_group.nginx.id
    broker = aws_security_group.broker.id
    runner = aws_security_group.runner.id
  }

  type                     = "ingress"
  from_port                = 443
  to_port                  = 443
  protocol                 = "tcp"
  source_security_group_id = each.value
  security_group_id        = aws_security_group.apne3_vpc_endpoint_for_ecs.id
  description              = "HTTPS from ${each.key}"
}

resource "aws_vpc_endpoint" "apne3_ecr_api" {
  vpc_id            = aws_vpc.apne3.id
  service_name      = "com.amazonaws.ap-northeast-3.ecr.api"
  vpc_endpoint_type = "Interface"

  subnet_ids         = aws_subnet.apne3_private[*].id
  security_group_ids = [aws_security_group.apne3_vpc_endpoint_for_ecs.id]

  private_dns_enabled = true

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3-ecr-api"
  })
}

resource "aws_vpc_endpoint" "apne3_ecr_dkr" {
  vpc_id            = aws_vpc.apne3.id
  service_name      = "com.amazonaws.ap-northeast-3.ecr.dkr"
  vpc_endpoint_type = "Interface"

  subnet_ids         = aws_subnet.apne3_private[*].id
  security_group_ids = [aws_security_group.apne3_vpc_endpoint_for_ecs.id]

  private_dns_enabled = true

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3-ecr-dkr"
  })
}

resource "aws_vpc_endpoint" "apne3_logs" {
  vpc_id            = aws_vpc.apne3.id
  service_name      = "com.amazonaws.ap-northeast-3.logs"
  vpc_endpoint_type = "Interface"

  subnet_ids         = aws_subnet.apne3_private[*].id
  security_group_ids = [aws_security_group.apne3_vpc_endpoint_for_ecs.id]

  private_dns_enabled = true

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3-logs"
  })
}

resource "aws_vpc_endpoint" "apne3_s3" {
  vpc_id       = aws_vpc.apne3.id
  service_name = "com.amazonaws.ap-northeast-3.s3"

  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.apne3_private.id]

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3-s3"
  })
}
