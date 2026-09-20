variable "aws_profile" {
  description = "AWS CLI profile for the aws provider so apply targets the intended account"
  type        = string

  validation {
    condition     = length(var.aws_profile) > 0
    error_message = "aws_profile must not be empty."
  }
}

variable "domain_name" {
  description = "Apex domain composing the AWS / Google Cloud region internal zones (<region>.<domain>) resolved across HA VPN"
  type        = string

  validation {
    condition = length(var.domain_name) <= 253 && alltrue([
      for label in split(".", var.domain_name) : length(label) <= 63
    ]) && can(regex("^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$", var.domain_name))
    error_message = "domain_name must be a lowercase DNS-1123 hostname with at least one dot; each label at most 63 characters and the full name at most 253 characters."
  }
}
