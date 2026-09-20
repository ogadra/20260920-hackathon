#!/bin/sh
# Create the bunshin-runners table in DynamoDB Local.
# Schema は broker/store の Repository 実装 (state-index を query する) に一致させる。
set -eu

ENDPOINT="http://dynamodb-local:8000"

OUTPUT=$(aws dynamodb create-table \
  --endpoint-url "$ENDPOINT" \
  --table-name bunshin-runners \
  --billing-mode PAY_PER_REQUEST \
  --attribute-definitions \
    AttributeName=runnerId,AttributeType=S \
    AttributeName=currentSessionId,AttributeType=S \
    AttributeName=state,AttributeType=S \
  --key-schema AttributeName=runnerId,KeyType=HASH \
  --global-secondary-indexes \
    'IndexName=session-index,KeySchema=[{AttributeName=currentSessionId,KeyType=HASH}],Projection={ProjectionType=ALL}' \
    'IndexName=state-index,KeySchema=[{AttributeName=state,KeyType=HASH},{AttributeName=runnerId,KeyType=RANGE}],Projection={ProjectionType=ALL}' \
  --region ap-northeast-1 2>&1) && echo "$OUTPUT" || {
  echo "$OUTPUT" >&2
  echo "$OUTPUT" | grep -q "ResourceInUseException"
}
