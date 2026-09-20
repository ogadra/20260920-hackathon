variable "domain_name" {
  description = "FQDN for the service"
  type        = string
}

variable "aws_profile" {
  description = "AWS CLI profile name for the target account"
  type        = string
}
