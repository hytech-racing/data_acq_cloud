# Build Stage
FROM golang:1.24-trixie AS go-builder

# Install required build dependencies
RUN apt-get update && apt-get install -y \
    libhdf5-dev \
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
FROM debian:trixie-slim AS runtime

# Install runtime dependencies. Only the shared libraries are needed here,
# not the -dev packages. ca-certificates is required for TLS to S3/MongoDB.
RUN apt-get update && apt-get install -y \
    libhdf5-310 \
    libhdf5-hl-310 \
    ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the server binary from the build stage
COPY --from=go-builder /app/cmd/cloud_webserver_v2/server .

# Declare runtime environment variables
ENV MONGODB_URI=""
ENV AWS_REGION=""
ENV AWS_S3_RUN_BUCKET=""
ENV AWS_S3_ENDPOINT=""
ENV AWS_ACCESS_KEY=""
ENV AWS_SECRET_KEY=""
ENV ENV=""

# Expose the port
EXPOSE 8080

ENTRYPOINT ["./server"]
