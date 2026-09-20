# Terraform operations with environment-specific tfvars
# Available environments: stg, prd

_validate-env env:
    @if [ "{{env}}" != "stg" ] && [ "{{env}}" != "prd" ]; then echo "Error: env must be 'stg' or 'prd', got '{{env}}'"; exit 1; fi

_validate-vendor vendor:
    @if [ "{{vendor}}" != "aws" ] && [ "{{vendor}}" != "archive" ]; then echo "Error: vendor must be 'aws' or 'archive', got '{{vendor}}'"; exit 1; fi

_validate-tf-backend-bucket:
    @if [ -z "${TF_BACKEND_BUCKET:-}" ]; then echo "Error: TF_BACKEND_BUCKET must be set (see .env.example)"; exit 1; fi

_validate-loadtest-scenario scenario:
    @if [ "{{scenario}}" != "session_uniqueness" ] && [ "{{scenario}}" != "concurrent_execute" ]; then echo "Error: scenario must be 'session_uniqueness' or 'concurrent_execute', got '{{scenario}}'"; exit 1; fi

_validate-loadtest-domain:
    @if [ -z "${LOADTEST_DOMAIN:-}" ]; then echo "Error: LOADTEST_DOMAIN must be set (see .env.example)"; exit 1; fi

# Initialize terraform with environment-specific S3 backend config
# Requires TF_BACKEND_BUCKET to be set (e.g. via direnv / .env)
init vendor env: (_validate-vendor vendor) (_validate-env env) _validate-tf-backend-bucket
    terraform -chdir=terraform/{{vendor}} init -reconfigure -backend-config="bucket=${TF_BACKEND_BUCKET}" -backend-config="key=bunshin/{{vendor}}/{{env}}.tfstate"

# Plan changes for the specified environment
plan vendor env: (_validate-vendor vendor) (_validate-env env)
    terraform -chdir=terraform/{{vendor}} plan -var-file=environments/{{env}}.tfvars

# Apply changes for the specified environment
apply vendor env: (_validate-vendor vendor) (_validate-env env)
    terraform -chdir=terraform/{{vendor}} apply -var-file=environments/{{env}}.tfvars

# Deploy services for the specified environment
deploy vendor env *service: (_validate-vendor vendor) (_validate-env env)
    scripts/{{vendor}}/deploy.sh {{env}} {{service}}

# Destroy resources for the specified environment
destroy vendor env: (_validate-vendor vendor) (_validate-env env)
    scripts/{{vendor}}/destroy.sh {{env}}

# Run a k6 load test scenario against https://${LOADTEST_DOMAIN} (see .env.example)
loadtest runner_count scenario: _validate-loadtest-domain (_validate-loadtest-scenario scenario)
    k6 run -e BASE_URL="https://${LOADTEST_DOMAIN}" -e RUNNER_COUNT={{runner_count}} loadtest/{{scenario}}.js 2>&1 | tee k6-output.log

# Check for session_id duplicates in k6 output (empty output means no duplicates)
loadtest-check-dup:
    grep 'SESSION_ID:' k6-output.log | sed 's/.*SESSION_ID://' | sort | uniq -d

# Count sessions per stack from k6 output (session_id is prefixed with the owning stack)
loadtest-stack-count:
    grep 'SESSION_ID:' k6-output.log | sed 's/.*SESSION_ID://; s/_.*//' | sort | uniq -c
