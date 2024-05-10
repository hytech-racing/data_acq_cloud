import os
import boto3
from dotenv import load_dotenv
import tempfile
from mcap.decoder import DecoderFactory
from mcap.reader import make_reader
from pymongo import MongoClient
import uuid


python_file_path: str = os.path.dirname(os.path.abspath(__file__))
os.chdir(python_file_path)
os.chdir('..')
os.chdir('env')
print(os.getcwd() + '/.env.dev')

load_dotenv(f"{os.getcwd()}/.env.dev")


def connect_to_database(db_url: str) -> MongoClient:
    return MongoClient(db_url, serverSelectionTimeoutMS=60000)  # May need to adjust the timeout if you can't connect

def get_database_collection(db_client: MongoClient, collection: str):
    hytech_database = db_client['hytechDB']
    return hytech_database[collection]


def populate_database():
    region_name: str | None = os.getenv('REGION_NAME')
    aws_access_key: str | None = os.getenv('AWS_ACCESS_KEY')
    aws_secret_access_key: str | None = os.getenv('AWS_PRIVATE_ACCESS_KEY')
    bucket: str | None = os.getenv('BUCKET_NAME')
    connection_url: str | None = os.getenv('DB_URL')

    if region_name is None or aws_access_key is None or aws_secret_access_key is None or bucket is None or connection_url is None:
        print('Cannot access environment variables')
        return

    mongo_client: MongoClient = connect_to_database(connection_url)
    runs_db = get_database_collection(mongo_client, 'run_data')

    s3_client = boto3.client(
        service_name='s3',
        region_name=region_name,
        aws_access_key_id=aws_access_key,
        aws_secret_access_key=aws_secret_access_key
    )

    print(bucket)
    response = dict(s3_client.list_objects_v2(Bucket=bucket, Prefix='examples/', Delimiter='/'))
    #print(response)
    #print(response['KeyCount'])

    for content in response['Contents']:
        if content['Key'] != 'examples/' and content['Key'].endswith('.mcap'):
            metadata_obj: dict[str, str] = download_and_parse_mcap(s3_client, bucket, content['Key'])
            runs_db.insert_one(metadata_obj)

    # with tempfile.TemporaryFile(mode='w+b') as f:
    #     s3_client.download_fileobj(bucket, 'mykey', f)


def download_and_parse_mcap(s3_client, bucket: str, object_path: str) -> dict[str, str]:
    metadata_obj: dict[str, str] = {
        "_id": str(uuid.uuid4()),
        'mcap_object_path': object_path,
        'matlab_object_path': object_path.replace('.mcap', '.mat')
    }
    with tempfile.TemporaryFile(mode='w+b') as file:
        s3_client.download_fileobj(bucket, object_path, file)
        file.seek(0)

        reader = make_reader(file, decoder_factories=[DecoderFactory()])

        for metadata in reader.iter_metadata():
            m_name = getattr(metadata, 'name')
            m_data = getattr(metadata, 'metadata')
            metadata_obj[m_name] = m_data
            print(m_name)

    print(metadata_obj)
    return metadata_obj


if __name__ == "__main__":
    populate_database()
