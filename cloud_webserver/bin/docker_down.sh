#!/bin/sh

# Step 1: Build Docker images with Docker Compose
echo "Shutting down the docker images..."
docker compose --profile $1 down

if [ $1 == "dev" ]; then
  docker stop local_hytechdb
fi

# Step 2: Prune old Docker images
echo "Pruning old Docker images..."
docker image prune --force --filter "label=cloud_webserver-flask=true"
