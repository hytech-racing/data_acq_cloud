#!/bin/sh


docker run -d \
  -p 27017:27017 \
  -e MONGO_INITDB_ROOT_USERNAME=username \
  -e MONGO_INITDB_ROOT_PASSWORD=password \
  -v metadata_volume:/local_hytechdb_meta_data \
  -v car_setup:/local_hytechdb_car_data \
  --name local_hytechdb \
  mongo

sleep 3

docker exec -it local_hytechdb mongosh --username username --password password --eval "use hytechDB;" --eval "db.createCollection('run_data')"
