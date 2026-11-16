# Direct-to-S3 uploads

Large MCAP files are not sent through the application server. The client asks the
server to sign part URLs, uploads the bytes directly to the S3 bucket, and then tells
the server to process the finished object.

```
Browser ──initiate──→ Cloud Webserver ──sign──→ (no file bytes)
Browser ─────────────file parts──────────────→ S3
Browser ──finalize──→ Cloud Webserver ──→ S3, FileProcessor
```

The `POST /uploads/*` endpoints require a valid session cookie
(`credentials: "include"` on the client) and return their JSON object directly.
`GET /uploads/limits` requires no session and uses the server's usual envelope.

The server generates the S3 key. Clients never choose it, and every key is scoped to
the signed-in user:

```
direct-uploads/<username>/<random-id>/<file-name>.mcap
```

Only `.mcap` files are accepted. The per-file limit is the server's configured upload
capacity (currently 20 GiB).

## Endpoints

All paths are also reachable under the `/api/v2` mount.

### `POST /uploads/initiate`

Starts a multipart upload and returns every presigned part URL in the same response.
The client makes one request per file.

Request:

```json
{
  "fileName": "run_2026-11-08.mcap",
  "contentType": "application/octet-stream",
  "size": 734003200
}
```

`contentType` defaults to `application/octet-stream` when empty.

Response:

```json
{
  "uploadId": "2~abc...",
  "key": "direct-uploads/dchen656/8Yb.../run_2026-11-08.mcap",
  "partSize": 67108864,
  "parts": [
    { "partNumber": 1, "url": "https://bucket.s3.amazonaws.com/...&partNumber=1" },
    { "partNumber": 2, "url": "https://bucket.s3.amazonaws.com/...&partNumber=2" }
  ]
}
```

The server chooses the part size: 64 MiB (`67108864` bytes). The part count is
`ceil(size / partSize)`, and part numbers run from 1 through that count. The 20 GiB
file cap yields at most 320 parts. Every part except the last must be at least 5 MiB,
which the 64 MiB part size always satisfies. The last part is the remainder and may be
smaller.

Every part URL is signed for 24 hours, the same lifetime as the upload session. The
client keeps them for the whole transfer. Retrying a failed part reuses the same URL.

The response carries `Cache-Control: no-store`.

If the server cannot create the multipart upload or sign a part URL, it returns 500
`failed to prepare upload`, discards the multipart upload, and does not register the
session.

### `POST /uploads/abort`

Discards an unfinished multipart upload and its uploaded parts. The request body is
`{ "key": ..., "uploadId": ... }`. A successful response is `{}`.

### `POST /uploads/finalize`

Tells S3 to assemble the parts and queues the finished MCAP for the same processing
pipeline used by `POST /mcaps/bulk_upload`. The response contains the file processor
job ID.

Request:

```json
{
  "key": "direct-uploads/dchen656/8Yb.../run_2026-11-08.mcap",
  "uploadId": "2~abc...",
  "parts": [
    { "partNumber": 1, "eTag": "\"9b2cf535f27731c974343645a3985328\"" },
    { "partNumber": 2, "eTag": "\"d41d8cd98f00b204e9800998ecf8427e\"" }
  ],
  "hidden": false
}
```

Collect the `ETag` response header from each part `PUT`. The server sorts the parts by
number, so the client can send them in any order. `uploadId` and a non-empty `parts`
list are both required.

Response:

```json
{ "jobId": "job_1762123456789012345" }
```

Finalize is idempotent while the server process is running. A retry returns the job ID
from the first successful call instead of processing the file twice. A retry after S3
assembled the parts skips assembly, even after a server restart, because the server
sees the assembled object in the bucket.

### `GET /uploads/limits`

Reports the file processor capacity. The response uses the usual envelope:

```json
{
  "message": null,
  "data": {
    "current_file_size": 0,
    "max_file_size": 21474836480,
    "available_file_size": 21474836480
  }
}
```

## Client flow

```js
async function uploadMultipart(file, concurrency = 4) {
  const contentType = file.type || "application/octet-stream";
  const init = await api("/uploads/initiate", {
    fileName: file.name,
    contentType,
    size: file.size,
  });

  const { partSize, parts } = init;
  const completed = new Array(parts.length);
  let next = 0;

  async function worker() {
    while (next < parts.length) {
      const { partNumber, url } = parts[next++];
      const start = (partNumber - 1) * partSize;
      const blob = file.slice(start, Math.min(start + partSize, file.size));
      const response = await fetch(url, { method: "PUT", body: blob });
      if (!response.ok) throw new Error(`part ${partNumber} failed: ${response.status}`);
      completed[partNumber - 1] = {
        partNumber,
        eTag: response.headers.get("ETag"),
      };
    }
  }

  try {
    await Promise.all(Array.from({ length: concurrency }, worker));
    return api("/uploads/finalize", {
      key: init.key,
      uploadId: init.uploadId,
      parts: completed,
    });
  } catch (error) {
    await api("/uploads/abort", { key: init.key, uploadId: init.uploadId }).catch(() => {});
    throw error;
  }
}
```

## Bucket CORS

The browser talks to S3 directly, so the bucket must allow the frontend origin to `PUT`
and must expose `ETag`. AWS CLI:

```sh
cat > cors.json <<'EOF'
[
  {
    "AllowedOrigins": ["https://hytech-racing.github.io", "http://localhost:5173"],
    "AllowedMethods": ["PUT"],
    "AllowedHeaders": ["*"],
    "ExposeHeaders": ["ETag"],
    "MaxAgeSeconds": 3000
  }
]
EOF
aws s3api put-bucket-cors --bucket hytech-runs --cors-configuration file://cors.json
```

For MinIO, save the same JSON and run `mc cors set <alias>/<bucket> cors.json`.

Part URLs point at `AWS_S3_ENDPOINT` when `USING_NON_AWS_COMPATIBLE_STORE` is set. That
endpoint must be reachable from the browser, not only from the server.

## Failure modes

| Status | Meaning |
| --- | --- |
| 400 | Invalid file name/type/size, malformed body, or a finalize request with a missing, mismatched, or invalid upload ID or part list |
| 401 | No valid session cookie |
| 404 | Unknown upload (expired, never issued, or owned by another user) |
| 409 | Finalize already in progress, or the upload session expired |
| 500 | S3 could not start or sign the upload, or the upload could not be assembled |
| 503 | The server has no room to stage more files; retry after uploads finish |

Upload sessions are kept in memory for 24 hours. The part URLs stay valid for the same
24 hours. An assembled multipart upload survives a server restart because the
server can rebuild the session from the key and sees the object in the bucket. An
in-progress multipart upload does not: its upload ID is lost with the process, so the
client has to start that upload again.
