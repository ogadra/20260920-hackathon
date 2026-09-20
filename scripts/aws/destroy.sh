#!/usr/bin/env bash
set -euo pipefail

ENV="${1:?env (stg|prd) required}"
ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"

"${ROOT_DIR}/scripts/aws/cleanup-ecspresso-services.sh" "${ENV}"
terraform -chdir="${ROOT_DIR}/terraform/aws" destroy -var-file="environments/${ENV}.tfvars"
