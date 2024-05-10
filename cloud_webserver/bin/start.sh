#!/bin/sh

# Assumes a local redis server is running

#start celery
celery -A celery_app worker --loglevel=info &

# start flask server
waitress-serve --port=8080 --call app:create_app