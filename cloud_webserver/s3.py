import os
import boto3


class S3Client:
    def __init__(self):
        region_name = os.getenv('REGION_NAME')
        aws_access_key = os.getenv('AWS_ACCESS_KEY')
        aws_secret_access_key = os.getenv('AWS_PRIVATE_ACCESS_KEY')
        self.s3_client = boto3.client(
            service_name='s3',
            region_name=region_name,
            aws_access_key_id=aws_access_key,
            aws_secret_access_key=aws_secret_access_key
        )

        self.bucket = os.getenv('BUCKET_NAME')

    def upload_file(self, file_path: str, object_path: str):
        self.s3_client.upload_file(file_path, self.bucket, object_path)

    def get_signed_url(self, obj_path: str):
        if obj_path is None or obj_path == '':
            return ""

        obj = self.s3_client.generate_presigned_url('get_object',
                                                    Params={'Bucket': self.bucket,
                                                            'Key': obj_path},
                                                    ExpiresIn=3600)

        return obj
