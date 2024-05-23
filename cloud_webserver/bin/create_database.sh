#!/bin/sh

# Get the directory path of the script
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
VENV_DIR="script_venv"

echo "Creating database in docker..."

docker run -d \
  -p 27017:27017 \
  -e MONGO_INITDB_ROOT_USERNAME=username \
  -e MONGO_INITDB_ROOT_PASSWORD=password \
  -v metadata_volume:/local_hytechdb_meta_data \
  -v car_setup:/local_hytechdb_car_data \
  --name local_hytechdb \
  mongo

sleep 3

if [ ! -d "$DIR/$VENV_DIR" ]; then
  echo "Creating virtual environment to run the generation script..."
  python3 -m venv $DIR/$VENV_DIR

  if [ ! -d "$DIR/$VENV_DIR" ]; then
    echo "Couldn't create a virtual environment"
    exit 1
  fi
fi

source $DIR/$VENV_DIR/bin/activate

pip install -r $DIR/requirements.txt

python3 "$DIR/populate_database.py"

deactivate

echo "Done setting up local database"
