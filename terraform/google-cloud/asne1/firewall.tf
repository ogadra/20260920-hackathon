# trivy:ignore:AVD-GCP-0074 -- Broad port range is the intent of a catchall deny rule
resource "google_compute_firewall" "deny_all_ingress" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name      = "bunshin-asne1-deny-all-ingress"
  network   = google_compute_network.bunshin.name
  direction = "INGRESS"
  priority  = 65534

  deny {
    protocol = "all"
  }

  source_ranges = ["0.0.0.0/0"]

  log_config {
    metadata = "INCLUDE_ALL_METADATA"
  }
}

resource "google_compute_firewall" "allow_proxy_and_healthcheck" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name      = "bunshin-asne1-allow-proxy-and-healthcheck"
  network   = google_compute_network.bunshin.name
  direction = "INGRESS"
  priority  = 1000

  allow {
    protocol = "tcp"
    ports    = [tostring(local.service_ports.nginx)]
  }

  source_ranges = [
    local.proxy_only_subnet_cidr,
    "35.191.0.0/16",
    "130.211.0.0/22",
  ]

  log_config {
    metadata = "INCLUDE_ALL_METADATA"
  }
}
