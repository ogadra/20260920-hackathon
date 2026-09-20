resource "google_certificate_manager_dns_authorization" "internal" {
  name     = local.internal_lb_name
  location = local.region
  domain   = local.internal_lb_hostname
  labels   = local.common_labels
}

resource "google_certificate_manager_certificate" "internal" {
  name     = local.internal_lb_name
  location = local.region
  labels   = local.common_labels

  managed {
    domains            = [local.internal_lb_hostname]
    dns_authorizations = [google_certificate_manager_dns_authorization.internal.id]
  }
}

# Gateway の dynamic IP は再作成で VIP が変わり DNS を壊すため NamedAddress で固定
resource "google_compute_address" "internal_lb" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name         = local.internal_lb_name
  region       = local.region
  subnetwork   = google_compute_subnetwork.workload.id
  address_type = "INTERNAL"
}
