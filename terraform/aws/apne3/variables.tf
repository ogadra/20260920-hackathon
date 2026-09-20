variable "alb_certificate_arn" {
  description = "ACM certificate ARN for the ALB HTTPS listeners"
  type        = string
  sensitive   = true

  validation {
    condition     = can(regex("^arn:aws:acm:ap-northeast-3:[0-9]{12}:certificate/.+", var.alb_certificate_arn))
    error_message = "alb_certificate_arn must be an ap-northeast-3 ACM certificate ARN."
  }
}

variable "domain_name" {
  description = "FQDN for the service"
  type        = string
}

variable "peer_vpc" {
  description = "Peer VPC for cross-region routing and internal DNS resolution"
  type = object({
    id                    = string
    region                = string
    cidr                  = string
    peering_connection_id = string
  })
}

variable "google_cloud_forwarder_subnet_cidrs" {
  description = "Google Cloud asne1/asne2 workload subnet CIDRs hosting Cloud DNS inbound forwarders; egress destination for the Route53 Resolver OUTBOUND endpoint"
  type        = list(string)

  validation {
    condition     = length(var.google_cloud_forwarder_subnet_cidrs) > 0 && alltrue([for c in var.google_cloud_forwarder_subnet_cidrs : can(cidrhost(c, 0))])
    error_message = "google_cloud_forwarder_subnet_cidrs must be a non-empty list of CIDR blocks."
  }
}

variable "google_cloud_dns_forwarder_source_range" {
  description = "Google-owned source CIDR for Cloud DNS forwarder queries hitting the Route53 Resolver INBOUND endpoint"
  type        = string

  validation {
    condition     = can(cidrhost(var.google_cloud_dns_forwarder_source_range, 0))
    error_message = "google_cloud_dns_forwarder_source_range must be a CIDR block."
  }
}
