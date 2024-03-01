import typing
from typing import Mapping, Any

from flask import Flask, request
from pymongo import MongoClient
from pymongo.collection import Collection
def save_metadata(path_to_file: str,
                  metadata_collection: Collection[Mapping[str, Any]],
                  car_setup_collection: Collection[Mapping[str, Any]],
                  metadata,
                  car_setup) -> None:
    insert_metadata_collection = metadata_collection.insert_one(metadata)
    insert_car_setup_collection = car_setup_collection.insert_one(car_setup)

    pass


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