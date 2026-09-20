resource "google_compute_ha_vpn_gateway" "bunshin" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name    = "bunshin-ha-vpn-asne1"
  region  = local.region
  network = google_compute_network.bunshin.id
}
