# Consumed by ecspresso via tfstate plugin for nginx INTERNAL_DOMAIN env var
output "domain_name" {
  description = "FQDN served by nginx, consumed at deploy render time"
  value       = var.domain_name
}

output "user_dns" {
  description = "DNS records to publish for var.domain_name in the external authoritative zone (NS1 + Route53 multi provider)"
  value = {
    addresses = {
      main = {
        name = var.domain_name
        # list型のまま出すとterraform outputがtolist([...])で表示し、権威zone側のtfvarsへそのまま貼れない。
        # familyをindexで引くと、DUAL_STACK化を含むplanではip_setsがIPv4しか返さず失敗する。
        a_records    = one([for ip_set in aws_globalaccelerator_accelerator.api_ingress.ip_sets : [for ip in ip_set.ip_addresses : ip] if ip_set.ip_family == "IPv4"])
        aaaa_records = one([for ip_set in aws_globalaccelerator_accelerator.api_ingress.ip_sets : [for ip in ip_set.ip_addresses : ip] if ip_set.ip_family == "IPv6"])
      }
    }
    aliases = {
      api_ingress_origin = {
        name    = local.api_ingress_origin_domain_name
        target  = aws_globalaccelerator_accelerator.api_ingress.dns_name
        zone_id = aws_globalaccelerator_accelerator.api_ingress.hosted_zone_id
      }
    }
  }
}
