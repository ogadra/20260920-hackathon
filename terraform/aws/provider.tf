# Terraform と AWS プロバイダのバージョン制約
terraform {
  required_version = ">= 1.15"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.52"
    }
  }
}

# AWS プロバイダ設定。環境ごとの tfvars で profile を切り替える
provider "aws" {
  region  = "ap-northeast-1"
  profile = var.aws_profile
}

provider "aws" {
  alias   = "apne1"
  region  = "ap-northeast-1"
  profile = var.aws_profile
}

provider "aws" {
  alias   = "apne3"
  region  = "ap-northeast-3"
  profile = var.aws_profile
}

# CloudFront に紐づく WAFv2 は us-east-1 でしか作成できない
provider "aws" {
  alias   = "use1"
  region  = "us-east-1"
  profile = var.aws_profile
}
