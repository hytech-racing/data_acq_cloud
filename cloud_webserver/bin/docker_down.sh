#!/bin/sh

# Step 1: Build Docker images with Docker Compose
echo "Shutting down the docker images..."
docker compose down

# Step 2: Prune old Docker images
echo "Pruning old Docker images..."
docker image prune --force --filter "label=cloud_webserver-flask=true"
