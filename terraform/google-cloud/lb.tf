resource "google_compute_health_check" "nginx" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name                = "bunshin-nginx"
  check_interval_sec  = 5
  timeout_sec         = 2
  healthy_threshold   = 2
  unhealthy_threshold = 3

  http_health_check {
    port         = 8080
    request_path = "/health"
  }
}

# /api/execute (runner)がSSE streamを返しchunked responseでcache不可なため、nginxを挟む/api/*
# 全体をCDN対象外にする。CDNはbackend bucket側で有効化する
resource "google_compute_backend_service" "nginx" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name                  = "bunshin-nginx"
  protocol              = "HTTP"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  timeout_sec           = 30
  enable_cdn            = false

  # 会場内NAT配下から同一送信元IPで大量requestが来る前提のため、affinityを張らず全podに分散させる。
  # 他stackセッションはnginx resolve.luaのbroker sub-request経由でfallbackさせる。
  # AWS Global Acceleratorのclient_affinity (未設定=NONE) と対称
  session_affinity = "NONE"

  security_policy      = google_compute_security_policy.backend.id
  edge_security_policy = google_compute_security_policy.edge.id

  custom_request_headers = ["X-Bunshin-Edge-Client-Address: {client_ip_address}:{client_port}"]

  health_checks = [google_compute_health_check.nginx.id]

  log_config {
    enable      = true
    sample_rate = 1.0
  }

  dynamic "backend" {
    for_each = concat(module.asne1.nginx_neg_ids, module.asne2.nginx_neg_ids)
    content {
      group                 = backend.value
      balancing_mode        = "RATE"
      max_rate_per_endpoint = 100
      capacity_scaler       = 1.0
    }
  }
}

resource "google_compute_url_map" "external" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name            = "bunshin-external"
  default_service = google_compute_backend_bucket.static.id

  host_rule {
    hosts        = [var.domain_name]
    path_matcher = "main"
  }

  # port-forward: {hex}.<stack>.<domain> は所有stackのnginx NEGだけを持つbackend serviceに
  # 振り分ける。cross-stack forwardはしない (nginx側でも404に落とす)
  host_rule {
    hosts        = ["*.asia-northeast1.${var.domain_name}"]
    path_matcher = "pf-asne1"
  }

  host_rule {
    hosts        = ["*.asia-northeast2.${var.domain_name}"]
    path_matcher = "pf-asne2"
  }

  path_matcher {
    name            = "main"
    default_service = google_compute_backend_bucket.static.id

    path_rule {
      paths   = ["/api/*"]
      service = google_compute_backend_service.nginx.id
    }
  }

  path_matcher {
    name            = "pf-asne1"
    default_service = google_compute_backend_service.nginx_pf_asne1.id
  }

  path_matcher {
    name            = "pf-asne2"
    default_service = google_compute_backend_service.nginx_pf_asne2.id
  }
}

# port-forwardのHostは所有stackにしか着弾させないため、backend NEGは当該stackのみ。
# nginx側のcross-stack fallbackや近接ルーティングは意図的に外す。
resource "google_compute_backend_service" "nginx_pf_asne1" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name                  = "bunshin-nginx-pf-asne1"
  protocol              = "HTTP"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  timeout_sec           = 30
  enable_cdn            = false

  security_policy      = google_compute_security_policy.backend.id
  edge_security_policy = google_compute_security_policy.edge.id

  health_checks = [google_compute_health_check.nginx.id]

  log_config {
    enable      = true
    sample_rate = 1.0
  }

  dynamic "backend" {
    for_each = module.asne1.nginx_neg_ids
    content {
      group                 = backend.value
      balancing_mode        = "RATE"
      max_rate_per_endpoint = 100
      capacity_scaler       = 1.0
    }
  }
}

resource "google_compute_backend_service" "nginx_pf_asne2" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name                  = "bunshin-nginx-pf-asne2"
  protocol              = "HTTP"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  timeout_sec           = 30
  enable_cdn            = false

  security_policy      = google_compute_security_policy.backend.id
  edge_security_policy = google_compute_security_policy.edge.id

  health_checks = [google_compute_health_check.nginx.id]

  log_config {
    enable      = true
    sample_rate = 1.0
  }

  dynamic "backend" {
    for_each = module.asne2.nginx_neg_ids
    content {
      group                 = backend.value
      balancing_mode        = "RATE"
      max_rate_per_endpoint = 100
      capacity_scaler       = 1.0
    }
  }
}

resource "google_compute_target_https_proxy" "external" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name            = "bunshin-external"
  url_map         = google_compute_url_map.external.id
  certificate_map = "//certificatemanager.googleapis.com/${google_certificate_manager_certificate_map.external.id}"
}

resource "google_compute_global_address" "external_ipv4" {
  name       = "bunshin-external-ipv4"
  ip_version = "IPV4"

  labels = local.common_labels
}

resource "google_compute_global_address" "external_ipv6" {
  name       = "bunshin-external-ipv6"
  ip_version = "IPV6"

  labels = local.common_labels
}

resource "google_compute_global_forwarding_rule" "external_ipv4" {
  name                  = "bunshin-external-ipv4"
  ip_address            = google_compute_global_address.external_ipv4.id
  ip_protocol           = "TCP"
  port_range            = "443"
  target                = google_compute_target_https_proxy.external.id
  load_balancing_scheme = "EXTERNAL_MANAGED"

  labels = local.common_labels
}

resource "google_compute_global_forwarding_rule" "external_ipv6" {
  name                  = "bunshin-external-ipv6"
  ip_address            = google_compute_global_address.external_ipv6.id
  ip_protocol           = "TCP"
  port_range            = "443"
  target                = google_compute_target_https_proxy.external.id
  load_balancing_scheme = "EXTERNAL_MANAGED"

  labels = local.common_labels
}
