locals {
  vpn_pairs = {
    apne1-asne1 = { aws = "apne1", google_cloud = "asne1" }
    apne1-asne2 = { aws = "apne1", google_cloud = "asne2" }
    apne3-asne1 = { aws = "apne3", google_cloud = "asne1" }
    apne3-asne2 = { aws = "apne3", google_cloud = "asne2" }
  }

  google_cloud_regions = {
    asne1 = "asia-northeast1"
    asne2 = "asia-northeast2"
  }

  aws_regions = {
    apne1 = "ap-northeast-1"
    apne3 = "ap-northeast-3"
  }

  # terraform/aws/locals.tfのgoogle_cloud_dns_forwarder_source_rangeと一致させる
  google_cloud_dns_forwarder_source_range = "35.199.192.0/19"

  # terraform/google-cloud/asne{1,2}/locals.tfのpods_secondary_cidrと一致させる
  google_cloud_pod_secondary_cidrs = ["10.2.16.0/20", "10.3.16.0/20"]

  google_cloud_internal_lb_cidrs = [
    for k in sort(keys(local.google_cloud_regions)) :
    "${data.google_compute_address.google_cloud_internal_lb[k].address}/32"
  ]

  # AWS VGW default ASN(apne1/apne3のaws_vpn_gatewayはamazon_side_asn未指定のため64512)
  aws_vgw_asn = 64512

  # Google Cloud Router ASN。regionごとに別ASNを採番して同一AWS側から見たBGP sessionを区別する
  google_cloud_router_asn = {
    asne1 = 64520
    asne2 = 64521
  }

  vpn_interfaces_all = { for e in flatten([
    for pk, pv in local.vpn_pairs : [
      for i in [0, 1] : {
        key          = "${pk}-if${i}"
        pair_key     = pk
        aws          = pv.aws
        google_cloud = pv.google_cloud
        interface    = i
      }
    ]
    ]) : e.key => e
  }

  vpn_interfaces_apne1 = { for k, v in local.vpn_interfaces_all : k => v if v.aws == "apne1" }
  vpn_interfaces_apne3 = { for k, v in local.vpn_interfaces_all : k => v if v.aws == "apne3" }

  vpn_tunnels_all = { for e in flatten([
    for ik, iv in local.vpn_interfaces_all : [
      for t in [1, 2] : {
        key           = "${ik}-t${t}"
        pair_key      = iv.pair_key
        interface_key = ik
        google_cloud  = iv.google_cloud
        interface     = iv.interface
        tunnel        = t
      }
    ]
    ]) : e.key => e
  }

  common_tags = {
    Project   = "Bunshin"
    ManagedBy = "terraform"
  }

  common_labels = {
    project    = "bunshin"
    managed_by = "terraform"
  }

  aws_vpn_connections = merge(
    aws_vpn_connection.apne1,
    aws_vpn_connection.apne3,
  )

  aws_inbound_endpoint_ips = {
    apne1 = data.aws_route53_resolver_endpoint.apne1_inbound.ip_addresses
    apne3 = data.aws_route53_resolver_endpoint.apne3_inbound.ip_addresses
  }

  # AWS defaultはtransform setが多く、SA payloadのfragmentationでGoogle Cloud HA VPNのrekeyが失敗する。
  # lifetimeはAWS上限(P1 28800 / P2 3600)を使う。Google Cloud HA VPNの固定値より短いためrekeyはAWS側発火。
  # https://cloud.google.com/network-connectivity/docs/vpn/how-to/connect-ha-vpn-aws-peer-gateway
  aws_vpn_ike_proposal = {
    ike_versions            = ["ikev2"]
    phase1_encryption       = ["AES256"]
    phase1_integrity        = ["SHA2-256"]
    phase1_dh_group         = [14]
    phase1_lifetime_seconds = 28800
    phase2_encryption       = ["AES256"]
    phase2_integrity        = ["SHA2-256"]
    phase2_dh_group         = [14]
    phase2_lifetime_seconds = 3600
  }
}
