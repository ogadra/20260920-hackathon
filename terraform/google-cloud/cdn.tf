# static bucketはdual-regionでregion固有ではないため、backend bucketもrootに置く。
# lb.tfのURL mapがdefault_serviceでこれを指し、`/api/*`のみnginx backend serviceへ振る
resource "google_compute_backend_bucket" "static" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name        = "bunshin-static"
  bucket_name = google_storage_bucket.static.name
  enable_cdn  = true

  edge_security_policy = google_compute_security_policy.edge.id

  cdn_policy {
    cache_mode  = "CACHE_ALL_STATIC"
    default_ttl = 5
    max_ttl     = 10
    client_ttl  = 10
  }
}
