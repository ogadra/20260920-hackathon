#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVICES=(broker nginx runner)
ECR_REGION="ap-northeast-1"

die() {
    echo "Error: $*" >&2
    exit 1
}

contains_service() {
    local candidate="${1:?}"
    local service

    for service in "${SERVICES[@]}"; do
        if [[ "${service}" == "${candidate}" ]]; then
            return 0
        fi
    done

    return 1
}

login_ecr() {
    local env_name="${1:?}"
    local aws_account_id="${2:?}"
    local registry="${aws_account_id}.dkr.ecr.${ECR_REGION}.amazonaws.com"

    aws --profile "${env_name}" --region "${ECR_REGION}" ecr get-login-password \
        | docker login --username AWS --password-stdin "${registry}"
}

fetch_tfstate() {
    local env_name="${1:?}"

    : "${TF_BACKEND_BUCKET:?TF_BACKEND_BUCKET must be set (tfstate bucket for ecspresso plugin)}"

    TFSTATE_PATH="$(mktemp)"
    export TFSTATE_PATH
    trap 'rm -f "${TFSTATE_PATH}"' EXIT
    aws --profile prd s3 cp \
        "s3://${TF_BACKEND_BUCKET}/bunshin/aws/${env_name}.tfstate" \
        "${TFSTATE_PATH}" >/dev/null
}

main() {
    local env_name="${1:?Usage: scripts/aws/deploy.sh <env> [service...]}"
    shift
    local aws_account_id
    local -a target_services=()
    local service
    local pid
    local pids=()
    local status=0

    if [[ $# -eq 0 ]]; then
        target_services=("${SERVICES[@]}")
    else
        for service in "$@"; do
            contains_service "${service}" || die "service must be one of: ${SERVICES[*]} (got '${service}')"
            target_services+=("${service}")
        done
    fi

    aws_account_id="$(aws --profile "${env_name}" sts get-caller-identity --query Account --output text)"

    if [[ -z "${aws_account_id}" || "${aws_account_id}" == "None" ]]; then
        die "failed to resolve AWS account id for profile ${env_name}"
    fi

    login_ecr "${env_name}" "${aws_account_id}"
    fetch_tfstate "${env_name}"

    # shellcheck disable=SC1091
    source "${ROOT_DIR}/deploy/aws/stacks.env"

    # shellcheck disable=SC1091
    source "${ROOT_DIR}/deploy/aws/environments/${env_name}.env"

    for service in "${target_services[@]}"; do
        "${ROOT_DIR}/scripts/aws/deploy/service.sh" "${service}" "${env_name}" "${aws_account_id}" &
        pids+=("$!")
    done

    for pid in "${pids[@]}"; do
        if ! wait "${pid}"; then
            status=1
        fi
    done

    return "${status}"
}

main "$@"
