# Build Stage
FROM golang:1.23 AS go-builder

# Install required build dependencies
RUN apt-get update && apt-get install -y \
    libhdf5-serial-dev \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

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
    libhdf5-serial-dev \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the server binary from the build stage
COPY --from=go-builder /app/cmd/cloud_webserver_v2/server .

# Read in build arguments and save them to .env
ARG MONGODB_URI
ARG AWS_REGION
ARG AWS_S3_RUN_BUCKET
ARG AWS_ACCESS_KEY
ARG AWS_SECRET_KEY
ARG MATLAB_URI

RUN echo "MONGODB_URI=${MONGODB_URI}" >> .env && \
    echo "AWS_REGION=${AWS_REGION}" >> .env && \
    echo "AWS_S3_RUN_BUCKET=${AWS_S3_RUN_BUCKET}" >> .env && \
    echo "AWS_ACCESS_KEY=${AWS_ACCESS_KEY}" >> .env && \
    echo "AWS_SECRET_KEY=${AWS_SECRET_KEY}" >> .env && \
    echo "MATLAB_URI=${MATLAB_URI}" >> .env

# Expose the port
EXPOSE 8080

ENTRYPOINT ["./server"]
