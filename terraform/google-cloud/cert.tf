resource "google_certificate_manager_dns_authorization" "apex" {
  name        = "bunshin-apex"
  description = "DNS authorization for Google-managed cert on apex domain"
  domain      = var.domain_name

  labels = local.common_labels

  depends_on = [google_project_service.apis["certificatemanager.googleapis.com"]]
}

resource "google_certificate_manager_certificate" "apex" {
  name        = "bunshin-apex"
  description = "Google-managed cert for apex domain, served via Global External ALB"
  scope       = "DEFAULT"

  managed {
    domains = [var.domain_name]
    dns_authorizations = [
      google_certificate_manager_dns_authorization.apex.id,
    ]
  }

  labels = local.common_labels
}

# ssl_certificates直付けはCertificate Manager certには不可なため、target_https_proxyの
# certificate_mapを介してattachする経路をここで用意する
resource "google_certificate_manager_certificate_map" "external" {
  name        = "bunshin-external"
  description = "Certificate map bound to Global External ALB target_https_proxy"

  labels = local.common_labels

  depends_on = [google_project_service.apis["certificatemanager.googleapis.com"]]
}

resource "google_certificate_manager_certificate_map_entry" "apex" {
  name         = "bunshin-apex"
  description  = "Serve apex cert for SNI-matched requests to apex hostname"
  map          = google_certificate_manager_certificate_map.external.name
  certificates = [google_certificate_manager_certificate.apex.id]
  hostname     = var.domain_name

  labels = local.common_labels
}

# port-forwardのwildcard証明書。ワイルドカードはDNS-01のみで発行できるため
# stackごとにdns_authorizationを持ち、_acme-challenge.<stack>.<domain>のCNAME検証を通す
resource "google_certificate_manager_dns_authorization" "port_forward_asne1" {
  name        = "bunshin-pf-asne1"
  description = "DNS authorization for *.asia-northeast1.<domain>"
  domain      = "asia-northeast1.${var.domain_name}"

  labels = local.common_labels

  depends_on = [google_project_service.apis["certificatemanager.googleapis.com"]]
}

resource "google_certificate_manager_dns_authorization" "port_forward_asne2" {
  name        = "bunshin-pf-asne2"
  description = "DNS authorization for *.asia-northeast2.<domain>"
  domain      = "asia-northeast2.${var.domain_name}"

  labels = local.common_labels

  depends_on = [google_project_service.apis["certificatemanager.googleapis.com"]]
}

resource "google_certificate_manager_certificate" "port_forward_asne1" {
  name        = "bunshin-pf-asne1"
  description = "Google-managed cert for *.asia-northeast1.<domain>"
  scope       = "DEFAULT"

  managed {
    domains            = ["*.asia-northeast1.${var.domain_name}"]
    dns_authorizations = [google_certificate_manager_dns_authorization.port_forward_asne1.id]
  }

  labels = local.common_labels
}

resource "google_certificate_manager_certificate" "port_forward_asne2" {
  name        = "bunshin-pf-asne2"
  description = "Google-managed cert for *.asia-northeast2.<domain>"
  scope       = "DEFAULT"

  managed {
    domains            = ["*.asia-northeast2.${var.domain_name}"]
    dns_authorizations = [google_certificate_manager_dns_authorization.port_forward_asne2.id]
  }

  labels = local.common_labels
}

resource "google_certificate_manager_certificate_map_entry" "port_forward_asne1" {
  name         = "bunshin-pf-asne1"
  description  = "Serve *.asia-northeast1.<domain> cert on SNI match"
  map          = google_certificate_manager_certificate_map.external.name
  certificates = [google_certificate_manager_certificate.port_forward_asne1.id]
  hostname     = "*.asia-northeast1.${var.domain_name}"

  labels = local.common_labels
}

resource "google_certificate_manager_certificate_map_entry" "port_forward_asne2" {
  name         = "bunshin-pf-asne2"
  description  = "Serve *.asia-northeast2.<domain> cert on SNI match"
  map          = google_certificate_manager_certificate_map.external.name
  certificates = [google_certificate_manager_certificate.port_forward_asne2.id]
  hostname     = "*.asia-northeast2.${var.domain_name}"

  labels = local.common_labels
}
