from celery_app import celery
import logging
import uuid
from mcap_handler import MCAPHandler
from s3 import S3Client
import mcap_to_mat as mcap_to_mats
from pymongo import MongoClient
from pymongo.collection import Collection
from typing import Mapping, Any
import db
import shutil
from typing import Any, Dict

logger = logging.getLogger(__name__)


def connect_to_database(db_url: str) -> MongoClient:
    return MongoClient(db_url)


def get_database_collection(db_client: MongoClient, collection: str) -> Collection[Mapping[str, Any]]:
    hytech_database = db_client['hytechDB']
    return hytech_database[collection]


@celery.task
def process_mcap(file_name: str,
                 path_to_uploaded_mcap: str,
                 db_url: str,
                 aws_region: str,
                 aws_access_key: str,
                 aws_secret_access_key: str,
                 aws_bucket: str):

    db_client = connect_to_database(db_url)
    run_data_collection = get_database_collection(db_client, 'run_data')

    if path_to_uploaded_mcap == '' or path_to_uploaded_mcap is None:
        logger.error('no mcap file path provided')
        return

    try:
        logger.info('started processing of ' + file_name)

        metadata_id = str(uuid.uuid4())

        mcap_handler = MCAPHandler(path_to_uploaded_mcap)
        # Front end will handle this instead.
        # mcap_handler.prepare_mcap()
        mcap_handler.parse_tire_pressure()
        path_to_mcap_file = mcap_handler.write_and_parse_metadata()
    except Exception as e:
        logger.error('could not parse the mcap file: ' + str(e))
        return

    try:
        mat_file_name = mcap_to_mats.parser(path_to_mcap_file)
        path_to_mat_file: str = f"files/{mat_file_name}"
    except Exception as e:
        logger.error('could not convert the mcap to mat: ' + str(e))
        return

    try:
        s3 = S3Client(region_name=aws_region,
                      aws_access_key=aws_access_key,
                      aws_secret_access_key=aws_secret_access_key,
                      aws_bucket=aws_bucket)

        # Date/Time format is going to be the same for all files
        formatted_date: str = file_name[0: 10].replace('_', '')

        mcap_object_path: str = f"{formatted_date}/{file_name}"

        s3.upload_file(file_path=path_to_mcap_file,
                       object_path=mcap_object_path)

        logger.info("uploaded mcap to s3")
    except Exception as e:
        logger.error('could not upload mcap file: ' + str(e))
        return

    try:

        matlab_object_path: str = f"{formatted_date}/{mat_file_name}"

        s3.upload_file(file_path=path_to_mat_file,
                       object_path=matlab_object_path)

        logger.info("uploaded matlab to s3")
    except Exception as e:
        logger.error('could not upload matlab file to s3: ' + str(e))
        return

    try:
        # Need to access and parse the mcap file
        # Once we know what data is in the mcap file, we can begin to parse it

        return_obj = db.save_metadata(run_data_collection,
                                      mcap_object_path,
                                      matlab_object_path,
                                      metadata_id,
                                      mcap_handler.metadata_obj)

        logger.info('uploaded object to database')

        shutil.rmtree("files")
    except Exception as e:
        logger.error('could not add document to the mongodb collection: ' + str(e))
        return

    logger.info('finished processing ' + file_name)
