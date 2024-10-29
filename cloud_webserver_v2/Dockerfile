# Build Stage
FROM golang:1.23-alpine AS go-builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
WORKDIR /app/cmd/cloud_webserver_v2

RUN go build -o server main.go

# Runtime Stage
FROM python:3.11.8-bookworm AS runtime

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY --from=go-builder /app/cmd/cloud_webserver_v2/server .

# The one python file
COPY internal/utils/matlab_file_creator.py internal/utils/

# wouldn't recognize .env without this line idk why
COPY .env .env

# ENV variables
ENV AWS_REGION=${AWS_REGION}
ENV AWS_S3_RUN_BUCKET=${AWS_S3_RUN_BUCKET}
ENV AWS_ACCESS_KEY=${AWS_ACCESS_KEY}
ENV AWS_SECRET_KEY=${AWS_SECRET_KEY}
ENV MONGO_URI=${MONGO_URI}

EXPOSE 8080

CMD ["./server"]
