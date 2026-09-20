terraform {
  required_version = ">= 1.15"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.52"
    }
  }
}

provider "aws" {
  alias   = "apne1"
  region  = "ap-northeast-1"
  profile = "prd"
}

provider "aws" {
  alias   = "apne3"
  region  = "ap-northeast-3"
  profile = "prd"
}
