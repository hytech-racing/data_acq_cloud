#!/bin/sh

# Get the directory path of the script
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

docker run -d \
  -p 27017:27017 \
  -e MONGO_INITDB_ROOT_USERNAME=username \
  -e MONGO_INITDB_ROOT_PASSWORD=password \
  -v metadata_volume:/local_hytechdb_meta_data \
  -v car_setup:/local_hytechdb_car_data \
  --name local_hytechdb \
  mongo

sleep 3


source "$DIR/../venv/bin/activate"

python3 "$DIR/populate_database.py"

deactivate