resource "google_artifact_registry_repository" "bunshin" {
  # checkov:skip=CKV_GCP_84:Google-managed encryption is sufficient
  location      = "asia-northeast1"
  repository_id = "bunshin"
  format        = "DOCKER"
  labels        = local.common_labels

  cleanup_policies {
    id     = "keep-last-3"
    action = "KEEP"
    most_recent_versions {
      keep_count = 3
    }
  }
}
