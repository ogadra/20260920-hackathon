output "network_self_link" {
  description = "Self link of the VPC network"
  value       = google_compute_network.bunshin.self_link
}

output "nginx_neg_ids" {
  description = "IDs of nginx standalone zonal NEGs backing the Global External ALB"
  value       = [for _, neg in data.google_compute_network_endpoint_group.nginx : neg.id]
}

output "nginx_resolver" {
  description = "kube-dns Service IP for nginx.conf resolver directive (10th host in the services secondary range)"
  value       = cidrhost(local.services_secondary_cidr, 10)
}

output "internal_acme_cname" {
  description = "ACME DNS-01 challenge CNAME for the internal LB cert (publish in the public DNS zone)"
  value = {
    name = google_certificate_manager_dns_authorization.internal.dns_resource_record[0].name
    data = google_certificate_manager_dns_authorization.internal.dns_resource_record[0].data
  }
}
