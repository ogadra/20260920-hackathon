resource "google_compute_network_peering" "asne1_to_asne2" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  provider = google.asne1

  name         = "bunshin-asne1-to-asne2"
  network      = module.asne1.network_self_link
  peer_network = module.asne2.network_self_link
}

resource "google_compute_network_peering" "asne2_to_asne1" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  provider = google.asne2

  name         = "bunshin-asne2-to-asne1"
  network      = module.asne2.network_self_link
  peer_network = module.asne1.network_self_link
}
