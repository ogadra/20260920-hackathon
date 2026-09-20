resource "google_project_service_identity" "logging" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  provider = google-beta

  service = "logging.googleapis.com"
}

# trivy:ignore:AVD-GCP-0066 -- Google-managed encryption is sufficient for logs already retained in Cloud Logging
# trivy:ignore:AVD-GCP-0077 -- Access logging on an archive bucket would itself generate logs to archive
resource "google_storage_bucket" "logs" {
  # checkov:skip=CKV_GCP_62:Access logging on an archive bucket would itself generate logs to archive
  name     = "bunshin-logs"
  location = "ASIA-NORTHEAST1"

  labels = local.common_labels

  versioning {
    enabled = true
  }

  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  deletion_policy             = "PREVENT"

  soft_delete_policy {
    retention_duration_seconds = 90 * 24 * 60 * 60
  }

  retention_policy {
    retention_period = 365 * 24 * 60 * 60
    is_locked        = false
  }

  lifecycle_rule {
    action {
      type = "AbortIncompleteMultipartUpload"
    }
    condition {
      age = 1
    }
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "google_storage_bucket_iam_member" "logs_logging_writer" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  bucket = google_storage_bucket.logs.name
  role   = "roles/storage.objectCreator"
  member = google_project_service_identity.logging.member
}
