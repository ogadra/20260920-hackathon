terraform {
  required_version = ">= 1.15"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.40"
    }
  }
}

# project は gcloud ADC / GOOGLE_CLOUD_PROJECT の環境から解決させ、project ID を tfvars 化しない
provider "google" {}

provider "google" {
  alias   = "asne1"
  region  = "asia-northeast1"
  project = data.google_project.current.project_id
}

provider "google" {
  alias   = "asne2"
  region  = "asia-northeast2"
  project = data.google_project.current.project_id
}

data "google_project" "current" {}

data "google_client_openid_userinfo" "me" {}
