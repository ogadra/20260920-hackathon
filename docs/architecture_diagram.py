"""Generate the Bunshin multi-region AWS architecture diagram."""

from pathlib import Path

from diagrams import Cluster, Diagram, Edge
from diagrams.aws.compute import ECS
from diagrams.aws.database import Dynamodb
from diagrams.aws.network import (
    ELB,
    Endpoint,
    GlobalAccelerator,
    InternetGateway,
    NATGateway,
    Route53,
)
from diagrams.aws.security import SecretsManager
from diagrams.custom import Custom
from diagrams.onprem.client import Users
from diagrams.onprem.network import Internet

CLUSTER_FONT = {"fontsize": "20", "fontname": "Sans-Serif Bold"}

GRAPH_ATTR = {
    "fontsize": "32",
    "bgcolor": "white",
    "pad": "0.8",
    "nodesep": "0.9",
    "ranksep": "1.7",
    "compound": "true",
    **CLUSTER_FONT,
}

NODE_ATTR = {
    "fontsize": "16",
    "fontname": "Sans-Serif Bold",
    "labelloc": "b",
    "imagepos": "tc",
}

EDGE_ATTR = {
    "fontsize": "16",
    "fontname": "Sans-Serif Bold",
}

HERE = Path(__file__).parent
OUTPUT_FILE = str(HERE / "bunshin_architecture")
NS1_ICON = str(HERE / "ns1_icon.png")


def aws_region(name: str, cidr: str, azs: str) -> dict[str, object]:
    with Cluster(name, graph_attr={**CLUSTER_FONT, "margin": "20"}):
        ddb = Dynamodb("DynamoDB\nbunshin-runners")
        secret = SecretsManager("Secrets Manager\nbunshin-jev-api-key")

        with Cluster(f"VPC {cidr}", graph_attr={**CLUSTER_FONT, "margin": "16"}):
            with Cluster("Public Subnets", graph_attr={**CLUSTER_FONT, "margin": "16"}):
                igw = InternetGateway("Internet Gateway")
                nat = NATGateway("NAT Gateway\nregional")

            with Cluster(f"Private Subnets ({azs})", graph_attr={**CLUSTER_FONT, "margin": "20"}):
                api_alb = ELB("API Ingress ALB\ninternal HTTPS")
                internal_alb = ELB("Internal ALB\nregional HTTPS")
                private_dns = Route53(f"Private DNS\n{name}.domain")

                with Cluster("ECS Cluster: bunshin", graph_attr={**CLUSTER_FONT, "margin": "24"}):
                    nginx = ECS("NGINX\nFargate / ARM64")
                    runner = ECS("Runner\nFargate / ARM64\ndebian-slim + bash")
                    broker = ECS("Broker\nFargate / ARM64")

                vpce_gateway = Endpoint("Gateway VPCE\nDynamoDB / S3")
                vpce_interface = Endpoint("Interface VPCE\nECR / Logs / Secrets Manager")

                api_alb >> Edge(label="static + /api/*") >> nginx
                internal_alb >> nginx
                private_dns >> internal_alb
                nginx >> Edge(label="proxy") >> runner
                nginx >> Edge(label="resolve") >> broker
                runner >> Edge(label="register", constraint="false") >> broker
                vpce_gateway >> Edge(style="invis") >> api_alb
                vpce_interface >> Edge(style="invis") >> internal_alb

            nat >> igw
            runner >> Edge(label="HTTPS egress", style="dashed", constraint="false") >> nat

        broker >> Edge(constraint="false") >> ddb
        vpce_interface >> Edge(label="GetSecretValue", style="dashed", constraint="false") >> secret

    return {
        "api_alb": api_alb,
        "internal_alb": internal_alb,
        "nginx": nginx,
        "broker": broker,
        "runner": runner,
        "private_dns": private_dns,
        "igw": igw,
    }


def main() -> None:
    with Diagram(
        "Bunshin - Multi-Region AWS Architecture",
        show=False,
        filename=OUTPUT_FILE,
        outformat="png",
        direction="TB",
        graph_attr=GRAPH_ATTR,
        node_attr=NODE_ATTR,
        edge_attr=EDGE_ATTR,
    ):
        users = Users("Clients")

        with Cluster(
            "Authoritative DNS (Active-Active)\napex weighted 50/50 + health check",
            graph_attr={**CLUSTER_FONT, "margin": "16", "rank": "same"},
        ):
            route53 = Route53("Route 53")
            ns1 = Custom("NS1", NS1_ICON)

        users >> route53
        users >> ns1

        jev = Internet("Jev (TypeSafe System One)\napi.typesafe.ai")

        with Cluster("AWS", graph_attr={**CLUSTER_FONT, "margin": "24", "bgcolor": "#FFF7EC"}):
            accelerator = GlobalAccelerator("Global Accelerator\napex static IPs")

            apne1 = aws_region("ap-northeast-1", "10.0.0.0/16", "1a / 1c / 1d")
            apne3 = aws_region("ap-northeast-3", "10.1.0.0/16", "3a / 3b / 3c")

            accelerator >> Edge(label="weight 128") >> apne1["api_alb"]
            accelerator >> Edge(label="weight 128") >> apne3["api_alb"]

            apne1["nginx"] >> Edge(label="fallback (VPC Peering)", style="dashed", constraint="false") >> apne3["internal_alb"]
            apne3["nginx"] >> Edge(label="fallback (VPC Peering)", style="dashed", constraint="false") >> apne1["internal_alb"]
            apne1["private_dns"] >> Edge(label="VPC peering DNS", style="dashed", constraint="false") >> apne3["private_dns"]

        route53 >> accelerator
        ns1 >> accelerator

        apne1["igw"] >> Edge(label="validate command", style="dashed", constraint="false") >> jev
        apne3["igw"] >> Edge(style="dashed", constraint="false") >> jev


if __name__ == "__main__":
    main()
