import json
import sys
from time import time_ns

from pymongo import MongoClient
from mcap.writer import Writer
from mcap.reader import make_reader

# open an mcap file and write some test meta data stuff to it


# The code snippet below iterates through a dictionary, checks if any of the 
# values are strings that can be converted to floats, and if so, converts them. 
# It uses the str.isdigit() method to check if the string represents a digit, but this method alone is 
# not sufficient to identify floats since it returns False for strings containing a decimal point. Therefore, 
# we'll also try to convert each string to a float inside a try-except block to
#  catch any values that cannot be converted to floats.
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

with open("test.mcap", "wb") as stream:
    writer = Writer(stream)

    writer.start()
    
    # im thinking that the name is the table that it goes into and the

    metadata_prim = {
        "driver": "Burdell", 
        "track_name": "test lot",
        "event_type": "straight",
        "mcap_data_path": "/path/to/mcap",
        "mat_data_path": "/path/to/mat",
        "start_time": "69",
        "end_time": "420",
        "CAR_SETUP_ID": "this_is_a_hash",
        "drivetrain_type": "amk_quad",
        "mass": "666",
        "wheelbase": "5.20",
        "firmware_rev": "asdfkjn23rnj23krjnsddv"
    }
    writer.add_metadata(name="METADATA_PRIMARY", data=metadata_prim)
   
    # writer.add_metadata(name="CAR_SETUP", data=car_setup_metadata)
    writer.finish()


client = MongoClient('mongodb://admin:password@localhost:27017/')
db = client['HyTech_database']
client.drop_database('HyTech_database')

with open("test.mcap", "rb") as f:
    reader = make_reader(f)
    # create one dictionary from all of the metadata in the
    for test in reader.iter_metadata():
        collection = db[test.name]
        entry = convert_to_floats(test.metadata)

        insert_result = collection.insert_one(entry)
        # Print the ID of the inserted document
        print(f"Document inserted with ID: {insert_result.inserted_id}")
        print(f"{test.name}: {test.metadata}")
    # print(record)