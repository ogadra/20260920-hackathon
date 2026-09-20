locals {
  region = "asia-northeast2"

  workload_subnet_cidr    = "10.3.0.0/24"
  pods_secondary_cidr     = "10.3.16.0/20"
  services_secondary_cidr = "10.3.32.0/26"
  proxy_only_subnet_cidr  = "10.3.64.0/24"

  common_labels = {
    project    = "bunshin"
    managed_by = "terraform"
  }

  # nginx / broker が listen する port と runner の container port。ECS locals の ecs_services と対称
  service_ports = {
    nginx  = 8080
    broker = 8080
    runner = 3000
  }

  internal_lb_name     = "bunshin-internal-${local.region}"
  internal_lb_hostname = "${local.region}.${var.domain_name}"

  nginx_neg_name    = "bunshin-nginx-${local.region}"
  nginx_neg_zones   = ["${local.region}-a", "${local.region}-b", "${local.region}-c"]
  gke_membership_id = "bunshin-asne2"
}
