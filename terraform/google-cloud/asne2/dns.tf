resource "google_dns_policy" "bunshin" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name                      = "bunshin-asne2"
  enable_logging            = true
  enable_inbound_forwarding = true

  networks {
    network_url = google_compute_network.bunshin.id
  }
}

# 自regionのVPCだけbindするとcross-region fallbackがDNS解決できない
resource "google_dns_managed_zone" "internal" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name        = "bunshin-internal-${local.region}"
  dns_name    = "${local.region}.${var.domain_name}."
  description = "bunshin internal private zone for ${local.region}"
  visibility  = "private"

  private_visibility_config {
    networks {
      network_url = google_compute_network.bunshin.id
    }
    networks {
      network_url = var.peer_vpc_network
    }
  }
}

resource "google_dns_record_set" "internal_apex" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name         = google_dns_managed_zone.internal.dns_name
  managed_zone = google_dns_managed_zone.internal.name
  type         = "A"
  ttl          = 5

  rrdatas = [google_compute_address.internal_lb.address]
}
