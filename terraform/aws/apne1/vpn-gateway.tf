resource "aws_vpn_gateway" "apne1" {
  vpc_id = aws_vpc.apne1.id

  tags = merge(local.common_tags, {
    Name    = "bunshin-vpn-apne1"
    Service = "bunshin-vpn-apne1"
  })
}
