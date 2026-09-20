resource "terraform_data" "nginx_neg_ready" {
  for_each = toset(local.nginx_neg_zones)

  triggers_replace = [local.nginx_neg_name]

  provisioner "local-exec" {
    interpreter = ["bash", "-c"]
    command     = <<-EOT
      for i in $(seq 1 60); do
        if gcloud compute network-endpoint-groups describe ${local.nginx_neg_name} \
          --zone=${each.value} \
          --project=${data.google_client_config.default.project} \
          >/dev/null 2>&1; then
          exit 0
        fi
        sleep 5
      done
      echo "nginx NEG ${local.nginx_neg_name} in ${each.value} did not appear within 300s" >&2
      exit 1
    EOT
  }
}

data "google_compute_network_endpoint_group" "nginx" {
  for_each = toset(local.nginx_neg_zones)
  name     = local.nginx_neg_name
  zone     = each.value

  depends_on = [terraform_data.nginx_neg_ready]
}
