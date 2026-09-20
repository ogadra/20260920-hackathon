locals {
  ecs_services = {
    nginx  = { port = 8080 }
    broker = { port = 8080 }
    runner = { port = 3000 }
  }

  common_tags = {
    Project   = "Bunshin"
    ManagedBy = "terraform"
  }

  api_ingress_origin_domain_name = "api-ingress.${var.domain_name}"

  # asne1 / asne2 workload subnetと同期する
  google_cloud_forwarder_subnet_cidrs = ["10.2.0.0/24", "10.3.0.0/24"]

  google_cloud_dns_forwarder_source_range = "35.199.192.0/19"
}
