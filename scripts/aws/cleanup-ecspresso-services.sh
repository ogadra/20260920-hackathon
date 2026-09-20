#!/usr/bin/env bash
set -euo pipefail

ENV="${1:?env (stg|prd) required}"
ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVICES=(broker nginx runner)
REGION_DIRS=(apne1 apne3)

: "${TF_BACKEND_BUCKET:?TF_BACKEND_BUCKET must be set (tfstate bucket for ecspresso plugin)}"

TFSTATE_PATH="$(mktemp)"
export TFSTATE_PATH
trap 'rm -f "${TFSTATE_PATH}"' EXIT

aws --profile prd s3 cp \
    "s3://${TF_BACKEND_BUCKET}/bunshin/aws/${ENV}.tfstate" \
    "${TFSTATE_PATH}" >/dev/null

# shellcheck disable=SC1091
source "${ROOT_DIR}/deploy/aws/stacks.env"

export AWS_PROFILE="${ENV}"
export IMAGE_TAG="destroy"
export BROKER_DESIRED_COUNT=0
export NGINX_DESIRED_COUNT=0
export RUNNER_DESIRED_COUNT=0

region_for() {
    case "$1" in
        apne1) echo "ap-northeast-1" ;;
        apne3) echo "ap-northeast-3" ;;
        *) echo "unknown region dir: $1" >&2; return 1 ;;
    esac
}

delete_service() {
    local region_dir="$1"
    local service="$2"
    local region status aws_out
    local config="${ROOT_DIR}/deploy/aws/${region_dir}/${service}/ecspresso.yml"

    region="$(region_for "${region_dir}")"

    if ! aws_out="$(aws --profile "${ENV}" --region "${region}" ecs describe-services \
        --cluster bunshin --services "bunshin-${service}" \
        --query 'services[0].status' --output text 2>&1)"; then
        if [[ "${aws_out}" == *ClusterNotFoundException* ]]; then
            printf '[%s] cluster gone, skipping %s\n' "${region_dir}" "${service}"
            return 0
        fi
        printf '[%s] describe-services failed for %s: %s\n' "${region_dir}" "${service}" "${aws_out}" >&2
        return 1
    fi
    status="${aws_out}"

    if [[ "${status}" != "ACTIVE" && "${status}" != "DRAINING" ]]; then
        printf '[%s] %s already gone (status=%s)\n' "${region_dir}" "${service}" "${status:-none}"
        return 0
    fi

    printf '[%s] deleting %s via ecspresso\n' "${region_dir}" "${service}"
    local delete_out
    if delete_out="$(ecspresso delete --config "${config}" --force --terminate 2>&1)"; then
        printf '%s\n' "${delete_out}"
        return 0
    fi
    printf '%s\n' "${delete_out}" >&2
    if [[ "${delete_out}" == *ClusterNotFoundException* ]]; then
        printf '[%s] cluster gone mid-delete, skipping %s\n' "${region_dir}" "${service}"
        return 0
    fi
    printf '[%s] failed to delete %s\n' "${region_dir}" "${service}" >&2
    return 1
}

pids=()
for region_dir in "${REGION_DIRS[@]}"; do
    for service in "${SERVICES[@]}"; do
        delete_service "${region_dir}" "${service}" &
        pids+=("$!")
    done
done

status=0
for pid in "${pids[@]}"; do
    wait "${pid}" || status=$?
done
exit "${status}"
