from typing import Mapping, Any

from flask import Flask, request
from pymongo import MongoClient
from pymongo.collection import Collection
from werkzeug.datastructures import FileStorage

import upload
import db

app = Flask(__name__) 

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
meta_data_collection: Collection[Mapping[str, Any]] = db['metadata']
car_setup_collection: Collection[Mapping[str, Any]] = db['car_setup']


# Add data to MongoDB route 
@app.route('/add_data', methods=['POST']) 
def add_data() -> str:
	# Get data from request 
	data = request.json 

	# Insert data into MongoDB 
	demo_collection.insert_one(data)

	return 'Data added to MongoDB'


@app.route('/save_mcap', methods=['POST'])
def save_mcap() -> str:
	if 'file' in request.files:
		try:
			path_to_file: str = upload.save_mcap_file(request.files['file'])
			if path_to_file != "":

				# Need to access and parse the mcap file
				# Once we know what data is in the mcap file, we can begin to parse it

				meta_data = {}
				car_setup = {}

				db.save_metadata(path_to_file,
								 meta_data_collection,
								 car_setup_collection,
								 meta_data,
								 car_setup)
		except ValueError as e:
			return 'fail: ' + str(e)

		return 'success'
	return 'fail: no file provided'


if __name__ == '__main__': 
	app.run() 
