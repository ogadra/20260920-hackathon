# trivy:ignore:AVD-GCP-0066 -- Google-managed encryption is sufficient for static assets
resource "google_storage_bucket" "static" {
  # checkov:skip=CKV_GCP_62:Access logging is not required until static delivery logging is defined
  name     = format("bunshin-static-%s-asia1", data.google_project.current.project_id)
  location = "ASIA1"
  rpo      = "ASYNC_TURBO"

  labels = local.common_labels

  versioning {
    enabled = true
  }

  soft_delete_policy {
    retention_duration_seconds = 0
  }

  lifecycle_rule {
    action {
      type = "Delete"
    }
    condition {
      days_since_noncurrent_time = 7
    }
  }

  lifecycle_rule {
    action {
      type = "AbortIncompleteMultipartUpload"
    }
    condition {
      age = 1
    }
  }

  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  website {
    main_page_suffix = "index.html"
    not_found_page   = "index.html"
  }

  depends_on = [google_project_service.apis["storage.googleapis.com"]]
}

# Granting allUsers instead would expose the bucket URL alongside the CDN path.
# The load balancing service agent appears only after a backend bucket exists.
# That is why this storage binding depends on a compute resource.
resource "google_storage_bucket_iam_member" "static_cdn_read" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  bucket = google_storage_bucket.static.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:service-${data.google_project.current.number}@https-lb.iam.gserviceaccount.com"

  depends_on = [google_compute_backend_bucket.static]
}
