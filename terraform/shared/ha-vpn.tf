resource "aws_customer_gateway" "apne1" {
  provider = aws.apne1
  for_each = local.vpn_interfaces_apne1

  bgp_asn    = local.google_cloud_router_asn[each.value.google_cloud]
  ip_address = data.google_compute_ha_vpn_gateway.google_cloud[each.value.google_cloud].vpn_interfaces[each.value.interface].ip_address
  type       = "ipsec.1"

  tags = merge(local.common_tags, {
    Name = "bunshin-cgw-${each.key}"
  })
}

resource "aws_customer_gateway" "apne3" {
  provider = aws.apne3
  for_each = local.vpn_interfaces_apne3

  bgp_asn    = local.google_cloud_router_asn[each.value.google_cloud]
  ip_address = data.google_compute_ha_vpn_gateway.google_cloud[each.value.google_cloud].vpn_interfaces[each.value.interface].ip_address
  type       = "ipsec.1"

  tags = merge(local.common_tags, {
    Name = "bunshin-cgw-${each.key}"
  })
}

resource "aws_vpn_connection" "apne1" {
  provider = aws.apne1
  for_each = local.vpn_interfaces_apne1

  vpn_gateway_id      = data.aws_vpn_gateway.apne1.id
  customer_gateway_id = aws_customer_gateway.apne1[each.key].id
  type                = "ipsec.1"
  static_routes_only  = false

  tunnel1_ike_versions                 = local.aws_vpn_ike_proposal.ike_versions
  tunnel1_phase1_encryption_algorithms = local.aws_vpn_ike_proposal.phase1_encryption
  tunnel1_phase1_integrity_algorithms  = local.aws_vpn_ike_proposal.phase1_integrity
  tunnel1_phase1_dh_group_numbers      = local.aws_vpn_ike_proposal.phase1_dh_group
  tunnel1_phase1_lifetime_seconds      = local.aws_vpn_ike_proposal.phase1_lifetime_seconds
  tunnel1_phase2_encryption_algorithms = local.aws_vpn_ike_proposal.phase2_encryption
  tunnel1_phase2_integrity_algorithms  = local.aws_vpn_ike_proposal.phase2_integrity
  tunnel1_phase2_dh_group_numbers      = local.aws_vpn_ike_proposal.phase2_dh_group
  tunnel1_phase2_lifetime_seconds      = local.aws_vpn_ike_proposal.phase2_lifetime_seconds

  tunnel2_ike_versions                 = local.aws_vpn_ike_proposal.ike_versions
  tunnel2_phase1_encryption_algorithms = local.aws_vpn_ike_proposal.phase1_encryption
  tunnel2_phase1_integrity_algorithms  = local.aws_vpn_ike_proposal.phase1_integrity
  tunnel2_phase1_dh_group_numbers      = local.aws_vpn_ike_proposal.phase1_dh_group
  tunnel2_phase1_lifetime_seconds      = local.aws_vpn_ike_proposal.phase1_lifetime_seconds
  tunnel2_phase2_encryption_algorithms = local.aws_vpn_ike_proposal.phase2_encryption
  tunnel2_phase2_integrity_algorithms  = local.aws_vpn_ike_proposal.phase2_integrity
  tunnel2_phase2_dh_group_numbers      = local.aws_vpn_ike_proposal.phase2_dh_group
  tunnel2_phase2_lifetime_seconds      = local.aws_vpn_ike_proposal.phase2_lifetime_seconds

  tags = merge(local.common_tags, {
    Name = "bunshin-vpn-${each.key}"
  })
}

resource "aws_vpn_connection" "apne3" {
  provider = aws.apne3
  for_each = local.vpn_interfaces_apne3

  vpn_gateway_id      = data.aws_vpn_gateway.apne3.id
  customer_gateway_id = aws_customer_gateway.apne3[each.key].id
  type                = "ipsec.1"
  static_routes_only  = false

  tunnel1_ike_versions                 = local.aws_vpn_ike_proposal.ike_versions
  tunnel1_phase1_encryption_algorithms = local.aws_vpn_ike_proposal.phase1_encryption
  tunnel1_phase1_integrity_algorithms  = local.aws_vpn_ike_proposal.phase1_integrity
  tunnel1_phase1_dh_group_numbers      = local.aws_vpn_ike_proposal.phase1_dh_group
  tunnel1_phase1_lifetime_seconds      = local.aws_vpn_ike_proposal.phase1_lifetime_seconds
  tunnel1_phase2_encryption_algorithms = local.aws_vpn_ike_proposal.phase2_encryption
  tunnel1_phase2_integrity_algorithms  = local.aws_vpn_ike_proposal.phase2_integrity
  tunnel1_phase2_dh_group_numbers      = local.aws_vpn_ike_proposal.phase2_dh_group
  tunnel1_phase2_lifetime_seconds      = local.aws_vpn_ike_proposal.phase2_lifetime_seconds

  tunnel2_ike_versions                 = local.aws_vpn_ike_proposal.ike_versions
  tunnel2_phase1_encryption_algorithms = local.aws_vpn_ike_proposal.phase1_encryption
  tunnel2_phase1_integrity_algorithms  = local.aws_vpn_ike_proposal.phase1_integrity
  tunnel2_phase1_dh_group_numbers      = local.aws_vpn_ike_proposal.phase1_dh_group
  tunnel2_phase1_lifetime_seconds      = local.aws_vpn_ike_proposal.phase1_lifetime_seconds
  tunnel2_phase2_encryption_algorithms = local.aws_vpn_ike_proposal.phase2_encryption
  tunnel2_phase2_integrity_algorithms  = local.aws_vpn_ike_proposal.phase2_integrity
  tunnel2_phase2_dh_group_numbers      = local.aws_vpn_ike_proposal.phase2_dh_group
  tunnel2_phase2_lifetime_seconds      = local.aws_vpn_ike_proposal.phase2_lifetime_seconds

  tags = merge(local.common_tags, {
    Name = "bunshin-vpn-${each.key}"
  })
}

resource "aws_vpn_gateway_route_propagation" "apne1" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne1
  for_each = toset(data.aws_route_tables.apne1_private.ids)

  vpn_gateway_id = data.aws_vpn_gateway.apne1.id
  route_table_id = each.value
}

resource "aws_vpn_gateway_route_propagation" "apne3" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne3
  for_each = toset(data.aws_route_tables.apne3_private.ids)

  vpn_gateway_id = data.aws_vpn_gateway.apne3.id
  route_table_id = each.value
}

resource "google_compute_router" "vpn" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  for_each = local.google_cloud_regions

  name    = "bunshin-vpn-${each.key}-router"
  region  = each.value
  network = data.google_compute_network.google_cloud[each.key].id

  bgp {
    asn               = local.google_cloud_router_asn[each.key]
    advertise_mode    = "CUSTOM"
    advertised_groups = ["ALL_SUBNETS"]

    advertised_ip_ranges {
      range       = local.google_cloud_dns_forwarder_source_range
      description = "Return path for Cloud DNS forwarder queries to AWS Route53 Resolver INBOUND"
    }
  }
}

resource "google_compute_external_vpn_gateway" "cross_cloud" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  for_each = local.vpn_pairs

  name            = "bunshin-cross-cloud-${each.key}"
  redundancy_type = "FOUR_IPS_REDUNDANCY"
  description     = "AWS VGW tunnel endpoints for ${each.key}"

  interface {
    id         = 0
    ip_address = local.aws_vpn_connections["${each.key}-if0"].tunnel1_address
  }
  interface {
    id         = 1
    ip_address = local.aws_vpn_connections["${each.key}-if0"].tunnel2_address
  }
  interface {
    id         = 2
    ip_address = local.aws_vpn_connections["${each.key}-if1"].tunnel1_address
  }
  interface {
    id         = 3
    ip_address = local.aws_vpn_connections["${each.key}-if1"].tunnel2_address
  }
}

resource "google_compute_vpn_tunnel" "cross_cloud" {
  for_each = local.vpn_tunnels_all

  name                            = "bunshin-vpn-${each.key}"
  region                          = local.google_cloud_regions[each.value.google_cloud]
  vpn_gateway                     = data.google_compute_ha_vpn_gateway.google_cloud[each.value.google_cloud].id
  vpn_gateway_interface           = each.value.interface
  peer_external_gateway           = google_compute_external_vpn_gateway.cross_cloud[each.value.pair_key].id
  peer_external_gateway_interface = each.value.interface * 2 + (each.value.tunnel - 1)
  router                          = google_compute_router.vpn[each.value.google_cloud].id
  ike_version                     = 2

  shared_secret = each.value.tunnel == 1 ? (
    local.aws_vpn_connections[each.value.interface_key].tunnel1_preshared_key
    ) : (
    local.aws_vpn_connections[each.value.interface_key].tunnel2_preshared_key
  )

  labels = local.common_labels
}

resource "google_compute_router_interface" "vpn" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  for_each = local.vpn_tunnels_all

  name       = "bunshin-vpn-${each.key}"
  router     = google_compute_router.vpn[each.value.google_cloud].name
  region     = local.google_cloud_regions[each.value.google_cloud]
  vpn_tunnel = google_compute_vpn_tunnel.cross_cloud[each.key].name

  ip_range = each.value.tunnel == 1 ? (
    format("%s/30", local.aws_vpn_connections[each.value.interface_key].tunnel1_cgw_inside_address)
    ) : (
    format("%s/30", local.aws_vpn_connections[each.value.interface_key].tunnel2_cgw_inside_address)
  )
}

resource "google_compute_router_peer" "vpn" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  for_each = local.vpn_tunnels_all

  name      = "bunshin-vpn-${each.key}"
  router    = google_compute_router.vpn[each.value.google_cloud].name
  region    = local.google_cloud_regions[each.value.google_cloud]
  interface = google_compute_router_interface.vpn[each.key].name
  peer_asn  = local.aws_vgw_asn

  peer_ip_address = each.value.tunnel == 1 ? (
    local.aws_vpn_connections[each.value.interface_key].tunnel1_vgw_inside_address
    ) : (
    local.aws_vpn_connections[each.value.interface_key].tunnel2_vgw_inside_address
  )
}
