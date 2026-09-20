resource "aws_vpn_gateway" "apne3" {
  vpc_id = aws_vpc.apne3.id

  tags = merge(local.common_tags, {
    Name    = "bunshin-vpn-apne3"
    Service = "bunshin-vpn-apne3"
  })
}
