FROM python:3.11.8

# Copy requirements file
COPY requirements.txt /app/requirements.txt

# Install dependencies
RUN pip install --no-cache-dir -r /app/requirements.txt

# Copy the rest of the application code
COPY . /app
COPY .env /app/.env

# Run your application
CMD ["python", "app.py"]
