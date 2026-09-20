terraform {
  backend "s3" {
    region       = "ap-northeast-1"
    encrypt      = true
    use_lockfile = true
    profile      = "prd"
  }
}
