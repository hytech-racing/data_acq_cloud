import typing
from typing import Mapping, Any
from pymongo.collection import Collection
import s3


def save_metadata(run_collection: Collection[Mapping[str, Any]],
                  path_to_mcap_file: str,
                  path_to_matlab_file: str,
                  document_id: str,
                  metadata: dict[str:str]) -> None:
    # TODO: handle path to matlab files, also figure out what we should query

    print(metadata)
    convert_to_floats(metadata['setup'])

    # Edit this whenever the front-end (data_acq/py_data_acq/py_data_acq/web_server/mcap_server.py) adds/edits/deletes what kind of metadata is processed
    run_data = {
        '_id': document_id,
        'date': metadata['setup']['date'],
        'driver': metadata['setup']['driver'],
        'track_name': metadata['setup']['trackName'],
        'event_type': metadata['setup']['eventType'],
        'drivetrain_type': metadata['setup']['drivetrainType'],
        'mass': metadata['setup']['mass'],
        'wheelbase': metadata['setup']['wheelbase'],
        'firmware_rev': metadata['setup']['firmwareRev'],
        'mcap_object_path': path_to_mcap_file,
        'matlab_object_path': path_to_matlab_file
    }

    run_collection.insert_one(run_data)


def query_runs(run_collection: Collection[Mapping[str, Any]], fields: typing.Dict) -> typing.List[typing.Dict[str, Any]]:

    s3_client = s3.S3Client()

    run_metadata: typing.List[typing.Dict] = list(run_collection.find(fields, {}))

    if run_metadata is None:
        return []

    for metadata in run_metadata:
        if "mcap_object_path" in metadata:
            metadata['mcap_download_link'] = s3_client.get_signed_url(metadata['mcap_object_path'])
        else:
            metadata['mcap_download_link'] = ''

        if "matlab_object_path" in metadata:
            metadata['matlab_download_link'] = s3_client.get_signed_url(metadata['matlab_object_path'])
        else:
            metadata['matlab_download_link'] = ''

    return run_metadata


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
