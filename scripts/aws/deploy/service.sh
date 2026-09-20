#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../../.." && pwd)"
ECR_REGION="ap-northeast-1"
REPLICA_REGION="ap-northeast-3"
REGION_DIRS=(apne1 apne3)

die() {
    echo "Error: $*" >&2
    exit 1
}

wait_for_replication() {
    local service="${1:?}"
    local env_name="${2:?}"
    local digest="${3:?}"
    local attempt

    echo "Waiting for ${service}@${digest} replication to ${REPLICA_REGION}"
    for attempt in {1..60}; do
        if aws --profile "${env_name}" --region "${REPLICA_REGION}" ecr describe-images \
            --repository-name "bunshin/${service}" \
            --image-ids "imageDigest=${digest}" \
            >/dev/null 2>&1; then
            echo "[${REPLICA_REGION}] ${service} digest visible after ${attempt} attempt(s)"
            return
        fi
        if (( attempt % 6 == 0 )); then
            echo "[${REPLICA_REGION}] ${service} still replicating (attempt ${attempt}/60)"
        fi
        sleep 5
    done
    echo "Error: ${service}@${digest} was not replicated to ${REPLICA_REGION}" >&2
    exit 1
}

deploy_to_region() {
    local service="${1:?}"
    local region_dir="${2:?}"
    local svc_upper="${service^^}"
    local desired_var="${svc_upper}_DESIRED_COUNT_${region_dir^^}"
    local desired_value="${!desired_var:-}"

    [[ -n "${desired_value}" ]] \
        || die "${desired_var} must be set (e.g. ${desired_var}=3)"
    export "${svc_upper}_DESIRED_COUNT=${desired_value}"

    echo "[${region_dir}] deploying ${service} via ecspresso"
    ecspresso deploy \
        --config "${ROOT_DIR}/deploy/aws/${region_dir}/${service}/ecspresso.yml" \
        --rollback-events=DEPLOYMENT_FAILURE
    echo "[${region_dir}] ${service} stable"
}

main() {
    local service="${1:?Usage: scripts/aws/deploy/service.sh <service> <env> <aws_account_id>}"
    local env_name="${2:?Usage: scripts/aws/deploy/service.sh <service> <env> <aws_account_id>}"
    local aws_account_id="${3:?Usage: scripts/aws/deploy/service.sh <service> <env> <aws_account_id>}"
    local image_tag
    local short_image_tag
    local registry="${aws_account_id}.dkr.ecr.${ECR_REGION}.amazonaws.com"
    local digest
    local region_dir
    local pids=()
    local pid
    local exit_code=0

    : "${TFSTATE_PATH:?TFSTATE_PATH must be set (local tfstate for ecspresso plugin)}"

    image_tag="$(git -C "${ROOT_DIR}" rev-parse HEAD)"
    short_image_tag="$(git -C "${ROOT_DIR}" rev-parse --short=7 HEAD)"

    echo "Deploying ${service} to ${env_name}"
    echo "[${service}] building image"
    docker buildx build \
        --platform linux/arm64 \
        -f "${ROOT_DIR}/${service}/Dockerfile" \
        -t "${registry}/bunshin/${service}:${image_tag}" \
        -t "${registry}/bunshin/${service}:${short_image_tag}" \
        --push \
        "${ROOT_DIR}"
    digest="$(aws --profile "${env_name}" --region "${ECR_REGION}" ecr describe-images \
        --repository-name "bunshin/${service}" \
        --image-ids "imageTag=${image_tag}" \
        --query 'imageDetails[0].imageDigest' \
        --output text)"
    echo "[${service}] pushed digest ${digest}"
    wait_for_replication "${service}" "${env_name}" "${digest}"

    export AWS_PROFILE="${env_name}"
    export IMAGE_TAG="${image_tag}"

    for region_dir in "${REGION_DIRS[@]}"; do
        deploy_to_region "${service}" "${region_dir}" &
        pids+=("$!")
    done
    for pid in "${pids[@]}"; do
        if ! wait "${pid}"; then
            exit_code=1
        fi
    done
    return "${exit_code}"
}

main "$@"
