FROM python:3.11.8-bookworm AS builder

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Runtime stage

WORKDIR /app

# Install dependencies
RUN pip install --no-cache-dir -r /app/requirements.txt

# Copy the rest of the application code
COPY . /app

# Run application
RUN chmod +x /app/bin/start.sh
CMD ["./bin/start.sh"]