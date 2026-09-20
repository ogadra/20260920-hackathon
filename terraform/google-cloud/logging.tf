resource "google_logging_project_bucket_config" "bunshin_logs" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  project        = data.google_project.current.project_id
  location       = "global"
  bucket_id      = "bunshin-logs"
  retention_days = 365
  locked         = true

  depends_on = [google_project_service.apis["logging.googleapis.com"]]
}

# auto-created な `_Default` sink は import + `unique_writer_identity` 揃えが必要になるため触らず、専用 sink を別名で作る
resource "google_logging_project_sink" "bunshin" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name                   = "bunshin"
  project                = data.google_project.current.project_id
  destination            = "logging.googleapis.com/${google_logging_project_bucket_config.bunshin_logs.id}"
  unique_writer_identity = true
}
