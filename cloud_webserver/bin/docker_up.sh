#!/bin/sh

# Step 1: Build Docker images with Docker Compose
echo "Building Docker images..."
docker compose --profile $1 up --build -d

docker start local_hytechdb
