import os
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
from datetime import date
import mcap_files.mcap_to_mat as mcap_to_mats

app = Flask(__name__)

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

@app.route('/test-streaming', methods=['GET'])
def test_streaming() -> Response:
    s3 = S3Client('us-east-1')
    obj = s3.get_signed_url("03_05_2024/run_mcap.mcap")

    return obj


@app.route('/save_run', methods=['POST'])
def save_mcap() -> str:
    if 'file' in request.files:
        try:
            file = request.files['file']
            path_to_mcap_file: str = upload.save_mcap_file(file)
            
            if path_to_mcap_file != "":

                metadata_id = str(uuid.uuid4())

                mcap_handler = MCAPHandler(path_to_mcap_file)
                mcap_handler.parse_tire_pressure()
                mcap_handler.write_and_parse_metadata()

                path_to_mat_file: str = mcap_to_mats.parser(path_to_mcap_file)

                s3 = S3Client()

                curr_date = date.today()
                formatted_date = curr_date.strftime("%m-%d-%Y")

                mcap_object_path = f"{formatted_date}/{file.filename}"

                s3.upload_file(file_path=path_to_mcap_file,
                               object_path=mcap_object_path)

                matlab_object_path = ""

                # Need to access and parse the mcap file
                # Once we know what data is in the mcap file, we can begin to parse it

                db.save_metadata(run_data_collection,
                                 mcap_object_path,
                                 matlab_object_path,
                                 metadata_id,
                                 mcap_handler.metadata_obj)
        except ValueError as e:
            return 'fail: ' + str(e)

        return 'success'
    return 'fail: no file provided'


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
