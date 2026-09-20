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
