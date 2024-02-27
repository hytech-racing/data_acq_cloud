from flask import Flask, request
from pymongo import MongoClient
from werkzeug.datastructures import FileStorage

import upload

app = Flask(__name__) 

# root route 
@app.route('/') 
def hello_world() -> str:
	return 'Hello, World!'

# Set up MongoDB connection and collection 
client = MongoClient('mongodb://localhost:27017/') 

# Create database named demo if they don't exist already 
db = client['demo'] 

# Create collection named data if it doesn't exist already 
collection = db['data'] 

# Add data to MongoDB route 
@app.route('/add_data', methods=['POST']) 
def add_data() -> str:
	# Get data from request 
	data = request.json 

	# Insert data into MongoDB 
	collection.insert_one(data) 

	return 'Data added to MongoDB'


@app.route('/upload_mcap', methods=['POST'])
def upload_mcap() -> str:
	if 'file' in request.files:
		try:
			upload.save_mcap_file(request.files['file'])
		except ValueError as e:
			return 'fail: ' + str(e)

		return 'success'
	return 'fail: no file provided'


if __name__ == '__main__': 
	app.run() 
