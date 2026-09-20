locals {
  vpc_cidr             = "10.0.0.0/16"
  azs                  = ["ap-northeast-1a", "ap-northeast-1c", "ap-northeast-1d"]
  public_cidrs         = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  private_cidrs        = ["10.0.11.0/24", "10.0.12.0/24", "10.0.13.0/24"]
  private_ipv6_netnums = [11, 12, 13]

  ecs_services = {
    nginx  = { port = 8080 }
    broker = { port = 8080 }
    runner = { port = 3000 }
  }

  # port-forwardで外部から届けたいrunner内アプリのlisten port。
  # 既定のrunner API (:3000) とは別のSGルールで参照する。
  runner_app_port = 5000

  api_ingress_port_forward_port = 8443

  ecr_repository_arns = {
    for service in keys(local.ecs_services) :
    service => "arn:aws:ecr:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:repository/bunshin/${service}"
  }

  common_tags = {
    Project   = "Bunshin"
    ManagedBy = "terraform"
  }
}
