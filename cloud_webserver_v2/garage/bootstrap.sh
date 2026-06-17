#!/bin/bash

set -euo pipefail

ADMIN_API="http://localhost:3902"
ADMIN_TOKEN="admin"
BUCKET="dops-dev"
ACCESS_KEY_NAME="dev"

header=(-H "Authorization: Bearer ${ADMIN_TOKEN}")

echo "==> Getting node ID..."
node_status=$(curl -s "${header[@]}" "${ADMIN_API}/v1/status")
node_id=$(echo "$node_status" | jq -r '.node')

echo "==> Assigning layout..."
curl -s "${header[@]}" -X POST "${ADMIN_API}/v1/layout" \
  -H "Content-Type: application/json" \
  -d "[{\"id\": \"${node_id}\", \"zone\": \"dc1\", \"capacity\": 1073741824, \"tags\": []}]" > /dev/null

echo "==> Applying layout..."
layout=$(curl -s "${header[@]}" "${ADMIN_API}/v1/layout")
version=$(echo "$layout" | jq -r '.version')
next_version=$((version + 1))
curl -s "${header[@]}" -X POST "${ADMIN_API}/v1/layout/apply" \
  -H "Content-Type: application/json" \
  -d "{\"version\": ${next_version}}" > /dev/null

echo "==> Creating bucket '${BUCKET}'..."
curl -s "${header[@]}" -X POST "${ADMIN_API}/v1/bucket" \
  -H "Content-Type: application/json" \
  -d "{\"globalAlias\": \"${BUCKET}\"}" > /dev/null

echo "==> Creating access key '${ACCESS_KEY_NAME}'..."
key_result=$(curl -s "${header[@]}" -X POST "${ADMIN_API}/v1/key" \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"${ACCESS_KEY_NAME}\"}")
key_id=$(echo "$key_result" | jq -r '.accessKeyId')
key_secret=$(echo "$key_result" | jq -r '.secretAccessKey')

echo "==> Granting key read/write on bucket..."
bucket_result=$(curl -s "${header[@]}" "${ADMIN_API}/v1/bucket?globalAlias=${BUCKET}")
bucket_id=$(echo "$bucket_result" | jq -r '.id')
curl -s "${header[@]}" -X POST "${ADMIN_API}/v1/bucket/allow" \
  -H "Content-Type: application/json" \
  -d "{\"bucketId\": \"${bucket_id}\", \"accessKeyId\": \"${key_id}\", \"permissions\": {\"read\": true, \"write\": true, \"owner\": true}}" > /dev/null

echo ""
echo "Garage is ready! Set these in your .env:"
echo "  AWS_S3_ENDPOINT=http://garage:3900"
echo "  AWS_ACCESS_KEY=${key_id}"
echo "  AWS_SECRET_KEY=${key_secret}"
echo "  AWS_S3_RUN_BUCKET=${BUCKET}"
echo "  AWS_REGION=us-east-1"
