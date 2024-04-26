#!/bin/sh

# Step 1: Build Docker images with Docker Compose
echo "Building Docker images..."
docker compose up --build -d
