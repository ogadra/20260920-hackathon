output "system" {
  description = "Values wired into k8s manifests by deploy.sh; not intended for humans"
  value = {
    broker_gsa_email = google_service_account.broker.email
    domain_name      = var.domain_name
    nginx_resolver = {
      asne1 = module.asne1.nginx_resolver
      asne2 = module.asne2.nginx_resolver
    }
    static_bucket = google_storage_bucket.static.name
  }
}

output "user_dns" {
  description = "DNS records to publish for var.domain_name in the operator's DNS provider"
  value = {
    a_record    = google_compute_global_address.external_ipv4.address
    aaaa_record = google_compute_global_address.external_ipv6.address
    acme_cnames = {
      apex = {
        name = google_certificate_manager_dns_authorization.apex.dns_resource_record[0].name
        data = google_certificate_manager_dns_authorization.apex.dns_resource_record[0].data
      }
      internal_asne1 = module.asne1.internal_acme_cname
      internal_asne2 = module.asne2.internal_acme_cname
      port_forward_asne1 = {
        name = google_certificate_manager_dns_authorization.port_forward_asne1.dns_resource_record[0].name
        data = google_certificate_manager_dns_authorization.port_forward_asne1.dns_resource_record[0].data
      }
      port_forward_asne2 = {
        name = google_certificate_manager_dns_authorization.port_forward_asne2.dns_resource_record[0].name
        data = google_certificate_manager_dns_authorization.port_forward_asne2.dns_resource_record[0].data
      }
    }
  }
}
