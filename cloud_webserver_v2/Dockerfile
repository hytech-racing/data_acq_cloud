# Build Stage
FROM ubuntu:22.04 AS go-builder

# Install Go and required build dependencies
RUN apt-get update && apt-get install -y \
    golang-1.23 \
    libhdf5-serial-dev \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Set up Go environment
ENV PATH=$PATH:/usr/lib/go-1.23/bin
WORKDIR /app

# Download Go module dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code and build the binary
COPY . .
WORKDIR /app/cmd/cloud_webserver_v2
RUN go build -o server main.go

# Runtime Stage
FROM ubuntu:22.04 AS runtime

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    python3.11 \
    python3-pip \
    libhdf5-serial-dev \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Install Python dependencies
COPY requirements.txt .
RUN pip3 install --no-cache-dir -r requirements.txt

# Copy the server binary from the build stage
COPY --from=go-builder /app/cmd/cloud_webserver_v2/server .

# Copy the Python script
COPY internal/utils/matlab_file_creator.py internal/utils/

# Copy the environment file
COPY .env .env

# Set environment variables
ENV AWS_REGION=${AWS_REGION}
ENV AWS_S3_RUN_BUCKET=${AWS_S3_RUN_BUCKET}
ENV AWS_ACCESS_KEY=${AWS_ACCESS_KEY}
ENV AWS_SECRET_KEY=${AWS_SECRET_KEY}
ENV MONGO_URI=${MONGO_URI}

# Expose the port
EXPOSE 8080

# Command to run the server
CMD ["./server"]
