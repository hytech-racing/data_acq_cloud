#!/bin/bash
set -e

cd "$(dirname "$0")"

# --- MongoDB ---
if [ "$(docker ps -f name=^my_mongo$ --format '{{.Names}}')" = "my_mongo" ]; then
  echo "MongoDB already running"
elif [ "$(docker ps -a -f name=^my_mongo$ --format '{{.Names}}')" = "my_mongo" ]; then
  echo "Restarting existing MongoDB container..."
  docker start my_mongo >/dev/null
else
  echo "Starting MongoDB..."
  docker run --rm -d \
    --name my_mongo \
    -e MONGO_INITDB_ROOT_USERNAME=admin \
    -e MONGO_INITDB_ROOT_PASSWORD=password \
    -p 27017:27017 \
    mongo:latest
fi

# --- MinIO (local S3 substitute) ---
if [ "$(docker ps -f name=^hytech_minio$ --format '{{.Names}}')" = "hytech_minio" ]; then
  echo "MinIO already running"
elif [ "$(docker ps -a -f name=^hytech_minio$ --format '{{.Names}}')" = "hytech_minio" ]; then
  echo "Restarting existing MinIO container..."
  docker start hytech_minio >/dev/null
  sleep 4
else
  echo "Starting MinIO..."
  docker run -d \
    --name hytech_minio \
    -e MINIO_ROOT_USER=minioadmin \
    -e MINIO_ROOT_PASSWORD=minioadmin \
    -p 9000:9000 -p 9001:9001 \
    -v hytech_minio_data:/data \
    minio/minio server /data --console-address ":9001"
  echo "Waiting for MinIO to come up..."
  sleep 4
fi

# Make sure the bucket the app expects actually exists (safe to re-run)
docker run --rm --entrypoint sh minio/mc -c \
  "mc alias set local http://host.docker.internal:9000 minioadmin minioadmin >/dev/null 2>&1 && mc mb local/hytech-dev >/dev/null 2>&1 || true"

# External volume the app container mounts for matlab/mps data
docker volume create matlab_mps_data >/dev/null 2>&1 || true

# --- App server (foreground, rebuilds on every run) ---
echo "Starting cloud_webserver_v2 on http://localhost:8081 ..."
docker compose -f docker/docker-compose_v2.yml up --build
