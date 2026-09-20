resource "google_firestore_database" "runners" {
  # checkov:skip=CKV_BUNSHIN_2:Resource does not support labels
  name                              = "bunshin-runners-asne2"
  location_id                       = "asia-northeast2"
  type                              = "FIRESTORE_NATIVE"
  concurrency_mode                  = "PESSIMISTIC"
  app_engine_integration_mode       = "DISABLED"
  point_in_time_recovery_enablement = "POINT_IN_TIME_RECOVERY_DISABLED"
  delete_protection_state           = "DELETE_PROTECTION_DISABLED"
  deletion_policy                   = "DELETE"
}
