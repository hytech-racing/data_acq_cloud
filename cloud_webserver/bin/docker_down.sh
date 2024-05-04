#!/bin/sh

# Step 1: Build Docker images with Docker Compose
echo "Shutting down the docker images..."
docker compose --profile $1 down

docker stop local_hytechdb

# Step 2: Prune old Docker images
echo "Pruning old Docker images..."
docker image prune --force --filter "label=cloud_webserver-flask=true"
