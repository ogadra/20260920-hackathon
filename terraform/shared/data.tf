data "aws_vpn_gateway" "apne1" {
  provider = aws.apne1

  filter {
    name   = "tag:Service"
    values = ["bunshin-vpn-apne1"]
  }
}

data "aws_vpn_gateway" "apne3" {
  provider = aws.apne3

  filter {
    name   = "tag:Service"
    values = ["bunshin-vpn-apne3"]
  }
}

data "aws_route_tables" "apne1_private" {
  provider = aws.apne1

  filter {
    name   = "tag:Name"
    values = ["bunshin-apne1-private"]
  }
}

data "aws_route_tables" "apne3_private" {
  provider = aws.apne3

  filter {
    name   = "tag:Name"
    values = ["bunshin-apne3-private"]
  }
}

data "google_compute_ha_vpn_gateway" "google_cloud" {
  for_each = local.google_cloud_regions

  name   = "bunshin-ha-vpn-${each.key}"
  region = each.value
}

data "google_compute_network" "google_cloud" {
  for_each = local.google_cloud_regions

  name = "bunshin-${each.key}-vpc"
}

data "aws_vpc" "apne1" {
  provider = aws.apne1

  filter {
    name   = "tag:Name"
    values = ["bunshin-apne1"]
  }
}

data "aws_vpc" "apne3" {
  provider = aws.apne3

  filter {
    name   = "tag:Name"
    values = ["bunshin-apne3"]
  }
}

data "aws_route53_resolver_endpoint" "apne1_inbound" {
  provider = aws.apne1

  filter {
    name   = "Name"
    values = ["bunshin-apne1-inbound"]
  }
}

data "aws_route53_resolver_endpoint" "apne1_outbound" {
  provider = aws.apne1

  filter {
    name   = "Name"
    values = ["bunshin-apne1-outbound"]
  }
}

data "aws_route53_resolver_endpoint" "apne3_inbound" {
  provider = aws.apne3

  filter {
    name   = "Name"
    values = ["bunshin-apne3-inbound"]
  }
}

data "aws_route53_resolver_endpoint" "apne3_outbound" {
  provider = aws.apne3

  filter {
    name   = "Name"
    values = ["bunshin-apne3-outbound"]
  }
}

data "google_compute_addresses" "google_cloud_inbound_forwarder" {
  for_each = local.google_cloud_regions

  region = each.value
  filter = "purpose = DNS_RESOLVER"
}

data "google_compute_address" "google_cloud_internal_lb" {
  for_each = local.google_cloud_regions

  name   = "bunshin-internal-${each.value}"
  region = each.value
}

data "aws_security_group" "apne1_nginx" {
  provider = aws.apne1
  vpc_id   = data.aws_vpc.apne1.id

  filter {
    name   = "tag:Name"
    values = ["bunshin-apne1-nginx"]
  }
}

data "aws_security_group" "apne3_nginx" {
  provider = aws.apne3
  vpc_id   = data.aws_vpc.apne3.id

  filter {
    name   = "tag:Name"
    values = ["bunshin-apne3-nginx"]
  }
}

data "aws_security_group" "apne1_internal_alb" {
  provider = aws.apne1
  vpc_id   = data.aws_vpc.apne1.id

  filter {
    name   = "tag:Name"
    values = ["bunshin-apne1-internal-alb"]
  }
}

data "aws_security_group" "apne3_internal_alb" {
  provider = aws.apne3
  vpc_id   = data.aws_vpc.apne3.id

  filter {
    name   = "tag:Name"
    values = ["bunshin-apne3-internal-alb"]
  }
}
