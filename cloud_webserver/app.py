import json
import os
import shutil
import typing
import uuid
from typing import Mapping, Any
from flask import Flask, request, Response
from pymongo import MongoClient
from pymongo.collection import Collection
from dotenv import load_dotenv
import upload
import db
from mcap_handler import MCAPHandler
from s3 import S3Client
import mcap_to_mat as mcap_to_mats
from flask_cors import CORS

app = Flask(__name__)
CORS(app, resources={r"/*": {"origins": "http://localhost:3000"}})  # Allow requests from http://localhost:3000
load_dotenv(dotenv_path="../.env")


# root route
@app.route('/')
def hello_world() -> str:
    return 'Hello, World!'


# Set up MongoDB connection and collection
db_client = MongoClient(os.environ.get('DB_URL'))

# Create database named demo if they don't exist already
hytech_database = db_client['hytechDB']

# Create collection named data if it doesn't exist already
run_data_collection: Collection[Mapping[str, Any]] = hytech_database['run_data']

@app.route('/save_run', methods=['POST'])
def save_mcap() -> Response:
    if 'file' not in request.files:
        return Response('No file provided', status=400)

    try:
        file = request.files['file']
        path_to_mcap_file: str = upload.save_mcap_file(file)

        if path_to_mcap_file != "":
            metadata_id = str(uuid.uuid4())

            mcap_handler = MCAPHandler(path_to_mcap_file)
            mcap_handler.parse_tire_pressure()
            mcap_handler.write_and_parse_metadata()

            mat_file_name = mcap_to_mats.parser(path_to_mcap_file)
            path_to_mat_file: str = f"files/{mat_file_name}"

            s3 = S3Client()

            formatted_date: str = mcap_handler.metadata_obj['setup']['date']

            mcap_object_path: str = f"{formatted_date}/{file.filename}"

            s3.upload_file(file_path=path_to_mcap_file,
                           object_path=mcap_object_path)

            matlab_object_path: str = f"{formatted_date}/{mat_file_name}"

            s3.upload_file(file_path=path_to_mat_file,
                           object_path=matlab_object_path)

            # Need to access and parse the mcap file
            # Once we know what data is in the mcap file, we can begin to parse it

            return_obj = db.save_metadata(run_data_collection,
                                          mcap_object_path,
                                          matlab_object_path,
                                          metadata_id,
                                          mcap_handler.metadata_obj)

            shutil.rmtree("files")
    except ValueError as e:
        return Response('fail: ' + str(e), status=500)

    return Response(json.dumps(return_obj), status=200, mimetype="application/json")


@app.route('/get_runs', methods=['POST'])
def get_runs() -> str | typing.List[typing.Dict[str, typing.Any]]:
    query = {}

    # Not worrying about random people sending random values to the server
    # because only the base station will access this because this will be on a secure vpn only we can access
    for key, value in request.form.items():
        query[key] = value

    get_runs_response: typing.List[typing.Dict[str, typing.Any]] = db.query_runs(run_data_collection, query)

    return get_runs_response


if __name__ == '__main__':
    app.run()
