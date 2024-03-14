import typing
from typing import Mapping, Any

from flask import Flask, request
from pymongo import MongoClient
from pymongo.collection import Collection
import uuid

def save_metadata(path_to_file: str,
                  metadata_collection: Collection[Mapping[str, Any]],
                  car_setup_collection: Collection[Mapping[str, Any]],
                  metadata) -> None:
    # insert_metadata_collection = metadata_collection.insert_one(metadata)



    # TODO: handle path to mcap and matlab files, also figure out what we should query


    # Edit this whenever the front-end (data_acq/py_data_acq/py_data_acq/web_server/mcap_server.py) adds/edits/deletes what kind of metadata is processed
    run_setup = {
        'driver': metadata['driver'],
        'track_name': metadata['trackName'],
        'event_type': metadata['eventType']
    }

    car_setup = {
        'drivetrain_type': metadata['drivetrainType'],
        'mass': convert_to_floats(metadata['mass']),
        'wheelbase': convert_to_floats(metadata['wheelbase']),
        'firmware_rev': metadata['firmwareRev']
    }

    car_setup_records = car_setup_collection.find({}, car_setup)
    car_setup_id = None
    if car_setup_records >= 1:
        car_setup_id = car_setup_records[0]['car_setup_id']
    else:
        car_setup_id = str(uuid.uuid4())

    run_setup['car_setup_id'] = car_setup_id
    car_setup['_id'] = car_setup_id

    metadata_collection.insert_one(run_setup)
    car_setup_collection.insert_one(car_setup)



# The code snippet below iterates through a dictionary, checks if any of the
# values are strings that can be converted to floats, and if so, converts them.
# It uses the str.isdigit() method to check if the string represents a digit, but this method alone is
# not sufficient to identify floats since it returns False for strings containing a decimal point. Therefore,
# we'll also try to convert each string to a float inside a try-except block to
#  catch any values that cannot be converted to floats. - courtesy of Ben
def convert_to_floats(input_dict):
    for key, value in input_dict.items():
        # Check if the value is a string
        if isinstance(value, str):
            # Try to convert the string to a float
            try:
                # This will succeed if the string represents a float
                converted_value = float(value)
                input_dict[key] = converted_value
            except ValueError:
                # This will happen if the string cannot be converted to a float
                # No action needed, just pass
                pass
    return input_dict