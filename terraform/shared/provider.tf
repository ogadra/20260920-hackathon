terraform {
  required_version = ">= 1.15"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.52"
    }
    google = {
      source  = "hashicorp/google"
      version = "~> 7.40"
    }
  }
}

# aws vendorと同型にvar.aws_profileで解決させ、backendのprd固定とapply対象accountの乖離を防ぐ
provider "aws" {
  region  = "ap-northeast-1"
  profile = var.aws_profile
}

# cross-cloud VPN は apne1/apne3 の VGW ごとに resource を作るため、region 別 alias を明示する
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

# projectはgcloud ADC / GOOGLE_CLOUD_PROJECTの環境から解決させ、project IDをtfvars化しない
provider "google" {}
