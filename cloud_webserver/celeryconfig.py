import os

broker_url = os.getenv('REDIS_URL', 'redis://redis:6379') # Need to have redis installed
result_backend = os.getenv('REDIS_URL', 'redis://redis:6379')
task_serializer = 'json'
result_serializer = 'json'
accept_content = ['json']
timezone = 'UTC'
