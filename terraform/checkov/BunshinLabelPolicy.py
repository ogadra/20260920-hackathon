"""Custom Checkov policy to enforce project=bunshin label on Google Cloud labelable resources."""

from typing import Any

from checkov.terraform.checks.resource.base_resource_check import BaseResourceCheck
from checkov.common.models.enums import CheckCategories, CheckResult


class BunshinLabelPolicy(BaseResourceCheck):
    """Ensure Google Cloud labelable resources have the project=bunshin label for cost management."""

    def __init__(self) -> None:
        """Initialize the check with ID, supported resources, and category."""
        name = "Ensure project=bunshin label is present on Google Cloud resources"
        check_id = "CKV_BUNSHIN_2"
        supported_resources = ["*"]
        categories = [CheckCategories.CONVENTION]
        super().__init__(name=name, id=check_id, categories=categories, supported_resources=supported_resources)

    def scan_resource_conf(self, conf: dict[str, list[Any]]) -> CheckResult:
        """Check that the Google Cloud resource has a project=bunshin label."""
        if not getattr(self, "entity_type", "").startswith("google_"):
            return CheckResult.PASSED
        # GKE 系 resource は API 側で `resource_labels` を使うため、そのフィールドも同義に扱う
        labels = conf.get("labels") or conf.get("resource_labels") or [{}]
        if isinstance(labels, list):
            labels = labels[0] if labels else {}
        if not isinstance(labels, dict):
            return CheckResult.UNKNOWN
        project_value = labels.get("project", "")
        if isinstance(project_value, list) and project_value:
            project_value = project_value[0]
        if project_value == "bunshin":
            return CheckResult.PASSED
        return CheckResult.FAILED


check = BunshinLabelPolicy()
