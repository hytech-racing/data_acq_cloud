broker_url = 'redis://redis:6379' # Need to have redis installed
result_backend = 'redis://redis:6379'
task_serializer = 'json'
result_serializer = 'json'
accept_content = ['json']
timezone = 'UTC'
