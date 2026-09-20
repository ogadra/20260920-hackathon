# 権威 DNS は別リポジトリが持つ。ここは公開が必要なレコードだけを出す。
output "user_dns" {
  description = "DNS records to publish for var.domain_name in the authoritative zone"
  value = {
    main = {
      name = var.domain_name
      # list型のまま出すとterraform outputがtolist([...])で表示し、権威zone側のtfvarsへそのまま貼れない。
      # familyをindexで引くと、DUAL_STACK化を含むplanではip_setsがIPv4しか返さず失敗する。
      a_records    = one([for ip_set in aws_globalaccelerator_accelerator.api_ingress.ip_sets : [for ip in ip_set.ip_addresses : ip] if ip_set.ip_family == "IPv4"])
      aaaa_records = one([for ip_set in aws_globalaccelerator_accelerator.api_ingress.ip_sets : [for ip in ip_set.ip_addresses : ip] if ip_set.ip_family == "IPv6"])
    }
  }
}
