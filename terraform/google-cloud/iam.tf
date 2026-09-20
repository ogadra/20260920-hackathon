resource "google_service_account" "broker" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  account_id   = "bunshin-broker"
  display_name = "bunshin broker (Workload Identity)"

  depends_on = [google_project_service.apis["iam.googleapis.com"]]
}

resource "google_project_iam_member" "broker_datastore_user" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  project = data.google_project.current.project_id
  role    = "roles/datastore.user"
  member  = google_service_account.broker.member

  depends_on = [google_project_service.apis["firestore.googleapis.com"]]
}

resource "google_project_iam_member" "deployer_gateway_editor" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  project = data.google_project.current.project_id
  role    = "roles/gkehub.gatewayEditor"
  member  = "user:${data.google_client_openid_userinfo.me.email}"

  depends_on = [google_project_service.apis["connectgateway.googleapis.com"]]
}
