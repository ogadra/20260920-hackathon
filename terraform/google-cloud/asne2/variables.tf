variable "broker_service_account_id" {
  description = "Broker GSA fully-qualified name (projects/-/serviceAccounts/<email>); the WI binding uses this to authorise the region's broker KSA (bunshin/broker-asne2)"
  type        = string

  validation {
    condition     = can(regex("^projects/[^/]+/serviceAccounts/[a-z][a-z0-9-]{4,28}[a-z0-9]@[a-z0-9-]+\\.iam\\.gserviceaccount\\.com$", var.broker_service_account_id))
    error_message = "broker_service_account_id must be projects/<project>/serviceAccounts/<gsa-email>."
  }
}

variable "domain_name" {
  description = "Apex domain used to compose the regional internal LB hostname (<region>.<domain>)"
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$", var.domain_name))
    error_message = "domain_name must be a lowercase DNS-1123 hostname with at least one dot."
  }
}

variable "peer_vpc_network" {
  description = "Peer region VPC self_link for cross-region private DNS visibility (asne2 zone must resolve from asne1 VPC and vice versa)"
  type        = string

  validation {
    condition     = can(regex("^https://www\\.googleapis\\.com/compute/v1/projects/[^/]+/global/networks/[^/]+$", var.peer_vpc_network))
    error_message = "peer_vpc_network must be a compute network self_link (https://www.googleapis.com/compute/v1/projects/<project>/global/networks/<name>)."
  }
}
