import json
import os
import typing
from typing import Mapping, Any, Dict
from flask import Flask, jsonify, request, Response
from pymongo import MongoClient
from pymongo.collection import Collection
from dotenv import load_dotenv
import upload
import db
from s3 import S3Client
from celery import Celery
from tasks import process_mcap

app = Flask(__name__)
app.config['CELERY_BROKER_URL'] = 'redis://redis:6379'
app.config['CELERY_RESULT_BACKEND'] = 'redis://redis:6379'

celery = Celery(app.name, broker=app.config['CELERY_BROKER_URL'])
celery.conf.update(app.config)

load_dotenv(dotenv_path=".env")
AWS_REGION = os.getenv('REGION_NAME')
AWS_ACCESS_KEY = os.getenv('AWS_ACCESS_KEY')
AWS_PRIVATE_ACCESS_KEY = os.getenv('AWS_PRIVATE_ACCESS_KEY')
AWS_BUCKET = os.getenv('BUCKET_NAME')

# root route
@app.route('/')
def hello_world() -> str:
    return 'Hello, World!'


DB_URL = os.environ.get('DB_URL')

# Set up MongoDB connection and collection
db_client = MongoClient(DB_URL)

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
        process_mcap.apply_async(args=[file.filename,
                                       path_to_mcap_file,
                                       DB_URL,
                                       AWS_REGION,
                                       AWS_ACCESS_KEY,
                                       AWS_PRIVATE_ACCESS_KEY,
                                       AWS_BUCKET])


    except ValueError as e:
        return Response('fail: ' + str(e), status=500)

    response = {'message': 'processing mcap'}
    json_response = json.dumps(response)
    return Response(json_response, status=200, mimetype="application/json")


@app.route('/get_runs', methods=['POST'])
def get_runs() -> str | typing.List[typing.Dict[str, typing.Any]]:
    query = {}

    # Not worrying about random people sending random values to the server
    # because only the base station will access this because this will be on a secure vpn only we can access
    for key, value in request.form.items():
        query[key] = value

    get_runs_response: typing.List[typing.Dict[str, typing.Any]] = db.query_runs(run_data_collection,
                                                                                 query,
                                                                                 AWS_REGION,
                                                                                 AWS_ACCESS_KEY,
                                                                                 AWS_PRIVATE_ACCESS_KEY,
                                                                                 AWS_BUCKET)

    return get_runs_response


# This route uses multipart/form-data to maintain consistency among the current routes in the server
@app.route('/get_offloaded_mcaps', methods=['POST'])
def get_offloaded_mcaps() -> str | typing.List[typing.Dict[str, typing.Any]]:
    s3 = S3Client(AWS_REGION, AWS_ACCESS_KEY, AWS_PRIVATE_ACCESS_KEY, AWS_BUCKET)

    mcap_offloaded_status: typing.Dict[str: typing.List[str]] = {"offloaded": [], "not_offloaded": []}

    for _, file_name in request.form.items():
        mcap_date = file_name[0: 10]
        mcap_date = mcap_date.replace("_", "-")
        offloaded: bool = s3.object_exists(f"{mcap_date}/{file_name}")
        if offloaded:
            mcap_offloaded_status["offloaded"].append(file_name)
        else:
            mcap_offloaded_status["not_offloaded"].append(file_name)

    return mcap_offloaded_status

def create_app():
    print(AWS_REGION)
    print(AWS_ACCESS_KEY)

    return app


if __name__ == '__main__':
    #serve(app, host="0.0.0.0", port=8080)
    print(AWS_REGION)
    print(AWS_ACCESS_KEY)
    app.run(host='0.0.0.0')
