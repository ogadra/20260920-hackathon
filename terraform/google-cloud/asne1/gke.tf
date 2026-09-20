# trivy:ignore:AVD-GCP-0061 -- IP endpoints are disabled entirely; master authorized networks does not apply
# trivy:ignore:AVD-GCP-0050 -- auto_provisioning_defaults.service_account references google_service_account.gke_node.email (managed in iam.tf); trivy cannot statically resolve provider-computed attributes
resource "google_container_cluster" "bunshin" {
  # checkov:skip=CKV_GCP_12:NetworkPolicy is enforced by Dataplane V2 on Autopilot; explicit network_policy block is not settable
  # checkov:skip=CKV_GCP_20:IP endpoints are disabled entirely; master authorized networks does not apply
  # checkov:skip=CKV_GCP_61:VPC Flow Logs are enabled on the workload subnet in P4-b; intranode visibility is managed by Autopilot and cannot be set explicitly
  # checkov:skip=CKV_GCP_65:RBAC binds Google identities directly (P4-h); Google Groups is optional and not adopted
  # checkov:skip=CKV_GCP_66:Binary Authorization is out of scope; image trust is managed via Artifact Registry lifecycle
  # checkov:skip=CKV_GCP_69:Autopilot enables the Workload Identity metadata server by default
  name             = "bunshin-asne1"
  location         = local.region
  enable_autopilot = true

  network    = google_compute_network.bunshin.id
  subnetwork = google_compute_subnetwork.workload.id

  ip_allocation_policy {
    cluster_secondary_range_name  = "pods"
    services_secondary_range_name = "services"
  }

  release_channel {
    channel = "STABLE"
  }

  # enable_private_endpoint is the legacy alias of control_plane_endpoints_config.ip_endpoints_config.enabled = false.
  # google provider 7.40 surfaces it as a perpetual "true -> null" diff when omitted.
  # Mirror it here to keep the plan clean.
  private_cluster_config {
    enable_private_nodes    = true
    enable_private_endpoint = true
  }

  # deploy / kubectl / kubernetes provider はすべて fleet 登録 + Connect Gateway 経由
  control_plane_endpoints_config {
    dns_endpoint_config {
      allow_external_traffic = true
    }
    ip_endpoints_config {
      enabled = false
    }
  }

  # 既存の VPC Flow Logs / Cloud DNS query log では L7 egress が見えないため有効化
  monitoring_config {
    enable_components = [
      "SYSTEM_COMPONENTS",
      "APISERVER",
      "SCHEDULER",
      "CONTROLLER_MANAGER",
      "STORAGE",
      "HPA",
      "POD",
      "DAEMONSET",
      "DEPLOYMENT",
      "STATEFULSET",
      "CADVISOR",
      "KUBELET",
    ]
    advanced_datapath_observability_config {
      enable_metrics = true
      enable_relay   = true
    }
  }

  cluster_autoscaling {
    auto_provisioning_defaults {
      service_account = google_service_account.gke_node.email
      oauth_scopes    = ["https://www.googleapis.com/auth/cloud-platform"]
      disk_type       = "pd-balanced"
      disk_size       = 10
    }
  }

  master_auth {
    client_certificate_config {
      issue_client_certificate = false
    }
  }

  resource_labels = local.common_labels

  deletion_protection = false
}

resource "google_gke_hub_membership" "bunshin" {
  membership_id = local.gke_membership_id

  endpoint {
    gke_cluster {
      resource_link = "//container.googleapis.com/${google_container_cluster.bunshin.id}"
    }
  }

  labels = local.common_labels
}
