resource "aws_route53_resolver_rule" "apne1_to_google_cloud" {
  provider = aws.apne1
  for_each = local.google_cloud_regions

  name                 = "bunshin-apne1-to-${each.key}"
  domain_name          = "${each.value}.${var.domain_name}"
  rule_type            = "FORWARD"
  resolver_endpoint_id = data.aws_route53_resolver_endpoint.apne1_outbound.id

  dynamic "target_ip" {
    for_each = data.google_compute_addresses.google_cloud_inbound_forwarder[each.key].addresses
    content {
      ip = target_ip.value.address
    }
  }

  tags = merge(local.common_tags, {
    Name = "bunshin-apne1-to-${each.key}"
  })
}

resource "aws_route53_resolver_rule" "apne3_to_google_cloud" {
  provider = aws.apne3
  for_each = local.google_cloud_regions

  name                 = "bunshin-apne3-to-${each.key}"
  domain_name          = "${each.value}.${var.domain_name}"
  rule_type            = "FORWARD"
  resolver_endpoint_id = data.aws_route53_resolver_endpoint.apne3_outbound.id

  dynamic "target_ip" {
    for_each = data.google_compute_addresses.google_cloud_inbound_forwarder[each.key].addresses
    content {
      ip = target_ip.value.address
    }
  }

  tags = merge(local.common_tags, {
    Name = "bunshin-apne3-to-${each.key}"
  })
}

resource "aws_route53_resolver_rule_association" "apne1" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne1
  for_each = local.google_cloud_regions

  name             = "bunshin-apne1-to-${each.key}"
  resolver_rule_id = aws_route53_resolver_rule.apne1_to_google_cloud[each.key].id
  vpc_id           = data.aws_vpc.apne1.id
}

resource "aws_route53_resolver_rule_association" "apne3" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne3
  for_each = local.google_cloud_regions

  name             = "bunshin-apne3-to-${each.key}"
  resolver_rule_id = aws_route53_resolver_rule.apne3_to_google_cloud[each.key].id
  vpc_id           = data.aws_vpc.apne3.id
}

# forwarding_path=privateでRFC1918宛でもpublic pathにfallbackさせずHA VPN経由に固定する
resource "google_dns_managed_zone" "aws_forwarding" {
  for_each = local.aws_regions

  name        = "bunshin-forward-${each.key}"
  dns_name    = "${each.value}.${var.domain_name}."
  description = "Forward AWS ${each.key} internal zone via HA VPN"
  visibility  = "private"
  labels      = local.common_labels

  private_visibility_config {
    networks {
      network_url = data.google_compute_network.google_cloud["asne1"].id
    }
    networks {
      network_url = data.google_compute_network.google_cloud["asne2"].id
    }
  }

  forwarding_config {
    dynamic "target_name_servers" {
      for_each = local.aws_inbound_endpoint_ips[each.key]
      content {
        ipv4_address    = target_name_servers.value
        forwarding_path = "private"
      }
    }
  }
}

# 到達元(AWS OUTBOUND ENI IP)がこのphaseで確定するためここで作る
resource "google_compute_firewall" "aws_outbound_dns" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  for_each = local.google_cloud_regions

  name      = "bunshin-${each.key}-allow-aws-outbound-dns"
  network   = data.google_compute_network.google_cloud[each.key].name
  direction = "INGRESS"
  priority  = 1000

  allow {
    protocol = "tcp"
    ports    = ["53"]
  }

  allow {
    protocol = "udp"
    ports    = ["53"]
  }

  source_ranges = concat(
    [for ip in data.aws_route53_resolver_endpoint.apne1_outbound.ip_addresses : "${ip}/32"],
    [for ip in data.aws_route53_resolver_endpoint.apne3_outbound.ip_addresses : "${ip}/32"],
  )

  log_config {
    metadata = "INCLUDE_ALL_METADATA"
  }
}
