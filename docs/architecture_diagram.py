"""Generate the Bunshin multi-cloud (AWS + Google Cloud) architecture diagram."""

from pathlib import Path

from diagrams import Cluster, Diagram, Edge
from diagrams.aws.compute import ECS
from diagrams.aws.database import Dynamodb
from diagrams.aws.network import (
    ELB,
    Endpoint,
    GlobalAccelerator,
    Route53,
    SiteToSiteVpn,
)
from diagrams.aws.security import WAF
from diagrams.custom import Custom
from diagrams.gcp.compute import KubernetesEngine
from diagrams.gcp.database import Firestore
from diagrams.gcp.network import CDN, DNS, LoadBalancing, Armor, VPN
from diagrams.gcp.storage import GCS
from diagrams.onprem.client import Users

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
        waf = WAF("AWS WAF\nREGIONAL ACL\napi-ingress ALB")
        ddb = Dynamodb("DynamoDB\nbunshin-runners")

        with Cluster(f"VPC {cidr}", graph_attr={**CLUSTER_FONT, "margin": "16"}):
            vpn = SiteToSiteVpn("Site-to-Site VPN\nVGW + BGP")

            with Cluster(f"Private Subnets ({azs})", graph_attr={**CLUSTER_FONT, "margin": "20"}):
                api_alb = ELB("API Ingress ALB\ninternal HTTPS")
                internal_alb = ELB("Internal ALB\nregional HTTPS")
                private_dns = Route53(f"Private DNS\n{name}.domain")

                with Cluster("ECS Cluster: bunshin", graph_attr={**CLUSTER_FONT, "margin": "24"}):
                    nginx = ECS("NGINX\n1 task / ARM64")
                    runner = ECS("Runner\nFargate / x86_64")
                    broker = ECS("Broker\n1 task / ARM64")

                vpce_gateway = Endpoint("Gateway VPCE\nDynamoDB")
                vpce_interface = Endpoint("Interface VPCE\nECR / Logs")

                api_alb >> Edge(label="static + /api/*") >> nginx
                internal_alb >> nginx
                private_dns >> internal_alb
                nginx >> Edge(label="proxy") >> runner
                nginx >> Edge(label="resolve") >> broker
                runner >> Edge(label="register", constraint="false") >> broker
                vpce_gateway >> Edge(style="invis") >> api_alb
                vpce_interface >> Edge(style="invis") >> internal_alb

        broker >> Edge(constraint="false") >> ddb
        waf >> Edge(style="invis") >> ddb

    return {
        "api_alb": api_alb,
        "internal_alb": internal_alb,
        "nginx": nginx,
        "broker": broker,
        "runner": runner,
        "private_dns": private_dns,
        "vpn": vpn,
    }


def gcp_region(name: str, project_region: str) -> dict[str, object]:
    with Cluster(name, graph_attr={**CLUSTER_FONT, "margin": "20"}):
        firestore = Firestore(f"Firestore Native\n{project_region}")

        with Cluster(f"VPC bunshin-{name}", graph_attr={**CLUSTER_FONT, "margin": "16"}):
            vpn = VPN("HA VPN Gateway\n+ Cloud Router (BGP)")

            with Cluster("Private Subnet (workload + proxy-only)", graph_attr={**CLUSTER_FONT, "margin": "20"}):
                rilb = LoadBalancing("Regional Internal LB\ngke-l7-rilb")
                private_dns = DNS(f"Cloud DNS private\n{project_region}.domain")

                with Cluster("GKE Autopilot: bunshin", graph_attr={**CLUSTER_FONT, "margin": "24"}):
                    nginx = KubernetesEngine("nginx Pod\nx86_64")
                    runner = KubernetesEngine("runner Pod\nx86_64")
                    broker = KubernetesEngine("broker Pod\nx86_64")

                rilb >> nginx
                private_dns >> rilb
                nginx >> Edge(label="proxy") >> runner
                nginx >> Edge(label="resolve") >> broker
                runner >> Edge(label="register", constraint="false") >> broker
                rilb >> Edge(style="invis") >> private_dns

        broker >> firestore

    return {
        "rilb": rilb,
        "nginx": nginx,
        "broker": broker,
        "runner": runner,
        "private_dns": private_dns,
        "vpn": vpn,
    }


def main() -> None:
    with Diagram(
        "Bunshin - Multi-Cloud Architecture (AWS + Google Cloud)",
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

        with Cluster("AWS", graph_attr={**CLUSTER_FONT, "margin": "24", "bgcolor": "#FFF7EC"}):
            accelerator = GlobalAccelerator("Global Accelerator\napex static IPs")

            apne1 = aws_region("ap-northeast-1", "10.0.0.0/16", "1a / 1c / 1d")
            apne3 = aws_region("ap-northeast-3", "10.1.0.0/16", "3a / 3b / 3c")

            accelerator >> Edge(label="weight 128") >> apne1["api_alb"]
            accelerator >> Edge(label="weight 128") >> apne3["api_alb"]

            apne1["nginx"] >> Edge(label="fallback (VPC Peering)", style="dashed", constraint="false") >> apne3["internal_alb"]
            apne3["nginx"] >> Edge(label="fallback (VPC Peering)", style="dashed", constraint="false") >> apne1["internal_alb"]
            apne1["private_dns"] >> Edge(label="VPC peering DNS", style="dashed", constraint="false") >> apne3["private_dns"]

        with Cluster("Google Cloud", graph_attr={**CLUSTER_FONT, "margin": "24", "bgcolor": "#EEF4FF"}):
            armor = Armor("Cloud Armor\nOWASP rules")
            global_lb = LoadBalancing("Global External LB\nAnycast IP")
            cloud_cdn = CDN("Cloud CDN")
            gcs = GCS("Cloud Storage\ndual-region asia1")

            armor >> global_lb
            global_lb >> Edge(label="PATH: / (static)") >> cloud_cdn >> gcs

            asne1 = gcp_region("asne1", "asia-northeast1")
            asne2 = gcp_region("asne2", "asia-northeast2")

            global_lb >> Edge(label="PATH: /api/*\nsessionAffinity=CLIENT_IP") >> asne1["nginx"]
            global_lb >> Edge(label="PATH: /api/*\nsessionAffinity=CLIENT_IP") >> asne2["nginx"]

            asne1["nginx"] >> Edge(label="fallback (VPC Peering)", style="dashed", constraint="false") >> asne2["rilb"]
            asne2["nginx"] >> Edge(label="fallback (VPC Peering)", style="dashed", constraint="false") >> asne1["rilb"]
            asne1["private_dns"] >> Edge(label="cross-region DNS", style="dashed", constraint="false") >> asne2["private_dns"]

        route53 >> Edge(label="AWS 50%") >> accelerator
        route53 >> Edge(label="GCP 50%") >> global_lb
        ns1 >> Edge(label="AWS 50%") >> accelerator
        ns1 >> Edge(label="GCP 50%") >> global_lb

        def vpn_edge(labeled: bool = False) -> Edge:
            return Edge(
                label="HA VPN mesh\n(BGP, apne1/apne3 x asne1/asne2 = 4 tunnels)" if labeled else "",
                style="dashed",
                color="#888888",
                constraint="false",
            )

        apne1["vpn"] >> vpn_edge(labeled=True) >> asne1["vpn"]
        apne1["vpn"] >> vpn_edge() >> asne2["vpn"]
        apne3["vpn"] >> vpn_edge() >> asne1["vpn"]
        apne3["vpn"] >> vpn_edge() >> asne2["vpn"]


if __name__ == "__main__":
    main()
