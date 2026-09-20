resource "google_compute_network" "bunshin" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name                    = "bunshin-asne2-vpc"
  auto_create_subnetworks = false
  routing_mode            = "REGIONAL"
}

resource "google_compute_subnetwork" "workload" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name          = "bunshin-asne2-workload"
  region        = local.region
  network       = google_compute_network.bunshin.id
  ip_cidr_range = local.workload_subnet_cidr
  purpose       = "PRIVATE"

  private_ip_google_access = true

  log_config {
    aggregation_interval = "INTERVAL_5_SEC"
    flow_sampling        = 1.0
    metadata             = "INCLUDE_ALL_METADATA"
  }

  secondary_ip_range {
    range_name    = "pods"
    ip_cidr_range = local.pods_secondary_cidr
  }

  secondary_ip_range {
    range_name    = "services"
    ip_cidr_range = local.services_secondary_cidr
  }
}

# trivy:ignore:AVD-GCP-0075 -- REGIONAL_MANAGED_PROXY subnets host Envoy proxies only and cannot enable Private Google Access
resource "google_compute_subnetwork" "proxy_only" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name          = "bunshin-asne2-proxy-only"
  region        = local.region
  network       = google_compute_network.bunshin.id
  ip_cidr_range = local.proxy_only_subnet_cidr
  purpose       = "REGIONAL_MANAGED_PROXY"
  role          = "ACTIVE"
}

resource "google_compute_router" "bunshin" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name    = "bunshin-asne2-router"
  region  = local.region
  network = google_compute_network.bunshin.id
}

resource "google_compute_router_nat" "bunshin" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name                               = "bunshin-asne2-nat"
  router                             = google_compute_router.bunshin.name
  region                             = local.region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"

  log_config {
    enable = true
    filter = "ERRORS_ONLY"
  }
}
