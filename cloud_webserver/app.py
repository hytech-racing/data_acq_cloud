import os
from typing import Mapping, Any

from flask import Flask, request, Response
from pymongo import MongoClient
from pymongo.collection import Collection
from werkzeug.datastructures import FileStorage
from dotenv import load_dotenv
import upload
import db
from cloud_webserver.parser import MCAPHandler
from cloud_webserver.s3 import S3Client

app = Flask(__name__)

load_dotenv(dotenv_path="../.env")

# root route
@app.route('/')
def hello_world() -> str:
    return 'Hello, World!'


# Set up MongoDB connection and collection
db_client = MongoClient('mongodb://localhost:27017/')

# Create database named demo if they don't exist already 
db = db_client['demo']

# Create collection named data if it doesn't exist already 
demo_collection = db['data']
run_data_collection: Collection[Mapping[str, Any]] = db['run_data']


# Add data to MongoDB route 
@app.route('/add_data', methods=['POST'])
def add_data() -> str:
    # Get data from request
    data = request.json

    # Insert data into MongoDB
    demo_collection.insert_one(data)

    return 'Data added to MongoDB'


@app.route('/test-streaming', methods=['GET'])
def test_streaming() -> Response:
    s3 = S3Client('us-east-1', 'run-metadata')
    obj = s3.get_signed_url("03_05_2024/run_mcap.mcap")

    return obj


@app.route('/save_mcap', methods=['POST'])
def save_mcap() -> str:
    if 'file' in request.files:
        try:
            path_to_file: str = upload.save_mcap_file(request.files['file'])
            if path_to_file != "":
                handler = MCAPHandler(path_to_file)
                handler.parse_tire_pressure()
                handler.write_and_parse_metadata()

                # Need to access and parse the mcap file
                # Once we know what data is in the mcap file, we can begin to parse it

                db.save_metadata(path_to_file,
                                 run_data_collection,
                                 handler.metadata_obj)
        except ValueError as e:
            return 'fail: ' + str(e)

        return 'success'
    return 'fail: no file provided'


if __name__ == '__main__':
    app.run()
