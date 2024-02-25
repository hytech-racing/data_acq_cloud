#!/bin/bash

# Define the container name
CONTAINER_NAME="my_mongo"

# Check if the MongoDB container is already running
if [ "$(docker ps -f name=$CONTAINER_NAME --format '{{.Names}}')" = "$CONTAINER_NAME" ]; then
  echo "MongoDB container '$CONTAINER_NAME' is already running."
else
  echo "Starting MongoDB container named '$CONTAINER_NAME'..."
  # Run the MongoDB container
  docker run --rm -d \
    --name $CONTAINER_NAME \
    -e MONGO_INITDB_ROOT_USERNAME=admin \
    -e MONGO_INITDB_ROOT_PASSWORD=password \
    -p 27017:27017 \
    mongo:latest
  echo "MongoDB container '$CONTAINER_NAME' has been started."
fi
