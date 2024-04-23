#!/bin/sh

# Step 1: Build Docker images with Docker Compose
echo "Building Docker images..."
docker compose up --build

# Step 2: Prune old Docker images
echo "Pruning old Docker images..."
docker image prune --force --filter "label=cloud_webserver-flask=true"