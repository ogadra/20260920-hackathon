resource "aws_vpc" "apne3" {
  # checkov:skip=CKV2_AWS_11:Flow logs are attached in the root module through the vpc_id output
  cidr_block                       = local.vpc_cidr
  assign_generated_ipv6_cidr_block = true
  enable_dns_hostnames             = true
  enable_dns_support               = true

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3"
  })
}

resource "aws_subnet" "apne3_public" {
  count = length(local.azs)

  vpc_id            = aws_vpc.apne3.id
  cidr_block        = local.public_cidrs[count.index]
  availability_zone = local.azs[count.index]

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3-public-${local.azs[count.index]}"
  })
}

resource "aws_subnet" "apne3_private" {
  count = length(local.azs)

  vpc_id            = aws_vpc.apne3.id
  cidr_block        = local.private_cidrs[count.index]
  ipv6_cidr_block   = cidrsubnet(aws_vpc.apne3.ipv6_cidr_block, 8, local.private_ipv6_netnums[count.index])
  availability_zone = local.azs[count.index]

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3-private-${local.azs[count.index]}"
  })
}

resource "aws_internet_gateway" "apne3" {
  vpc_id = aws_vpc.apne3.id

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3"
  })
}

resource "aws_nat_gateway" "apne3" {
  vpc_id            = aws_vpc.apne3.id
  availability_mode = "regional"

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3-nat"
  })

  depends_on = [aws_internet_gateway.apne3]
}

resource "aws_route_table" "apne3_public" {
  vpc_id = aws_vpc.apne3.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.apne3.id
  }

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3-public"
  })
}

resource "aws_route_table_association" "apne3_public" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  count = length(local.azs)

  subnet_id      = aws_subnet.apne3_public[count.index].id
  route_table_id = aws_route_table.apne3_public.id
}

resource "aws_route_table" "apne3_private" {
  # checkov:skip=CKV2_AWS_44:VPC peering route is limited to the peer VPC CIDR
  vpc_id = aws_vpc.apne3.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.apne3.id
  }

  route {
    cidr_block                = var.peer_vpc.cidr
    vpc_peering_connection_id = var.peer_vpc.peering_connection_id
  }

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3-private"
  })
}

resource "aws_route_table_association" "apne3_private" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  count = length(local.azs)

  subnet_id      = aws_subnet.apne3_private[count.index].id
  route_table_id = aws_route_table.apne3_private.id
}

resource "aws_default_security_group" "apne3" {
  vpc_id = aws_vpc.apne3.id

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3-default"
  })
}
