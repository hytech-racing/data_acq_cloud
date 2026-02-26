#!/usr/bin/env bash
set -euo pipefail

awslocal s3api create-bucket --bucket dops-dev || true
awslocal s3api create-bucket --bucket HT_proto || true
awslocal s3api create-bucket --bucket HT_CAN || true

