#!/bin/sh


# Get the directory path of the script
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Step 1: Build Docker images with Docker Compose
echo "Building Docker images..."

if [ "$1" == "dev" ]; then
	if [ ! "$(docker ps -a | grep local_hytechdb)" ]; then
		echo "You do not have a local hytech database! Creating one for you now: local_hytechdb"
		chmod +x /$DIR/create_database.sh

		/bin/bash /$DIR/create_database.sh
	fi

	echo "Starting your local database: local_hytechdb"
	docker start local_hytechdb
fi

docker compose --profile $1 up --build -d
