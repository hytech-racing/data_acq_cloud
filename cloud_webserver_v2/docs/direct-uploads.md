# Direct-to-S3 uploads

Large MCAP files are not sent through the application server. The client asks the
server to sign a request, uploads the bytes directly to the S3 bucket, and then tells
the server to process the finished object.

```
Browser ──presign/initiate/parts/complete──→ Cloud Webserver ──sign──→ (no file bytes)
Browser ──────────file bytes────────────────────────────────────────→ S3
Browser ──finalize─────────────────────────→ Cloud Webserver ──→ FileProcessor
```

Every endpoint below requires a valid session cookie (`credentials: "include"` on the
client) and returns the server's usual envelope:

```json
{ "message": null, "data": { } }
```

The server generates the S3 key. Clients never choose it, and every key is scoped to
the signed-in user:

```
direct-uploads/<username>/<random-id>/<file-name>.mcap
```

Only `.mcap` files are accepted. The per-file limit is the server's configured upload
capacity (currently 20 GiB).

## Endpoints

All paths are also reachable under the `/api/v2` mount.

### `POST /uploads/presign`

Use for files up to 5 GiB. Validates auth, file type and size, then returns a URL for a
single `PUT`.

Request:

```json
{
  "fileName": "run_2026-11-08.mcap",
  "contentType": "application/octet-stream",
  "size": 734003200
}
```

Response:

```json
{
  "message": null,
  "data": {
    "uploadUrl": "https://bucket.s3.amazonaws.com/direct-uploads/...",
    "key": "direct-uploads/dchen656/8Yb.../run_2026-11-08.mcap",
    "expiresIn": 300
  }
}
```

`Content-Type` is part of the signature. The `PUT` must send exactly the same value
that was used in the presign request. It also must be started within `expiresIn`
seconds; the transfer itself may take longer.

### `POST /uploads/initiate`

Starts a multipart upload. Use this for files over 5 GiB or when resumability matters.

Request: the same body as `/uploads/presign`.

Response:

```json
{
  "message": null,
  "data": {
    "uploadId": "2~abc...",
    "key": "direct-uploads/dchen656/8Yb.../run_2026-11-08.mcap"
  }
}
```

### `POST /uploads/parts`

Returns presigned URLs for one batch of parts. Request at most 100 part numbers per
call, and request them just before uploading so they do not expire while waiting.

Request:

```json
{
  "key": "direct-uploads/dchen656/8Yb.../run_2026-11-08.mcap",
  "uploadId": "2~abc...",
  "partNumbers": [1, 2, 3, 4]
}
```

Response:

```json
{
  "message": null,
  "data": {
    "parts": [
      { "partNumber": 1, "url": "https://bucket.s3.amazonaws.com/...&partNumber=1" },
      { "partNumber": 2, "url": "https://bucket.s3.amazonaws.com/...&partNumber=2" }
    ]
  }
}
```

Part numbers run from 1 to 10000. Every part except the last must be at least 5 MiB.
Part URLs are valid for 15 minutes.

### `POST /uploads/complete`

Tells S3 to assemble the uploaded parts. Collect the `ETag` response header from each
part `PUT` and send it back here. Parts are sorted by the server, so order does not
matter.

Request:

```json
{
  "key": "direct-uploads/dchen656/8Yb.../run_2026-11-08.mcap",
  "uploadId": "2~abc...",
  "parts": [
    { "partNumber": 1, "eTag": "\"9b2cf535f27731c974343645a3985328\"" },
    { "partNumber": 2, "eTag": "\"d41d8cd98f00b204e9800998ecf8427e\"" }
  ]
}
```

### `POST /uploads/abort`

Discards an unfinished multipart upload. The request body is `{ "key": ..., "uploadId": ... }`.

### `POST /uploads/finalize`

Confirms the upload landed in the bucket and queues the MCAP for the same processing
pipeline used by `POST /mcaps/bulk_upload`. The response contains the file processor
job ID; subscribe to `GET /mcaps/subscribe` to follow its progress.

Request:

```json
{ "key": "direct-uploads/dchen656/8Yb.../run_2026-11-08.mcap", "hidden": false }
```

Response:

```json
{ "message": null, "data": { "jobId": "job_1762123456789012345" } }
```

Finalize is idempotent: retrying it returns the job ID from the first successful call
instead of processing the file twice.

## Client flow

### Single PUT

```js
async function uploadWithPresignedPut(file) {
  const presign = await api("/uploads/presign", {
    fileName: file.name,
    contentType: file.type || "application/octet-stream",
    size: file.size,
  });

  const response = await fetch(presign.uploadUrl, {
    method: "PUT",
    headers: { "Content-Type": file.type || "application/octet-stream" },
    body: file,
  });
  if (!response.ok) throw new Error(`S3 upload failed: ${response.status}`);

  return api("/uploads/finalize", { key: presign.key });
}
```

### Multipart

```js
async function uploadMultipart(file, partSize = 64 * 1024 * 1024, concurrency = 4) {
  const contentType = file.type || "application/octet-stream";
  const init = await api("/uploads/initiate", {
    fileName: file.name,
    contentType,
    size: file.size,
  });

  const totalParts = Math.ceil(file.size / partSize);
  const partNumbers = Array.from({ length: totalParts }, (_, index) => index + 1);
  const urls = new Map();

  for (let index = 0; index < partNumbers.length; index += 100) {
    const batch = partNumbers.slice(index, index + 100);
    const { parts } = await api("/uploads/parts", {
      key: init.key,
      uploadId: init.uploadId,
      partNumbers: batch,
    });
    for (const part of parts) urls.set(part.partNumber, part.url);
  }

  const completed = new Array(totalParts);
  let next = 0;

  async function worker() {
    while (next < partNumbers.length) {
      const partNumber = partNumbers[next++];
      const start = (partNumber - 1) * partSize;
      const blob = file.slice(start, Math.min(start + partSize, file.size));
      const response = await fetch(urls.get(partNumber), { method: "PUT", body: blob });
      if (!response.ok) throw new Error(`part ${partNumber} failed: ${response.status}`);
      completed[partNumber - 1] = {
        partNumber,
        eTag: response.headers.get("ETag"),
      };
    }
  }

  try {
    await Promise.all(Array.from({ length: concurrency }, worker));
    await api("/uploads/complete", {
      key: init.key,
      uploadId: init.uploadId,
      parts: completed,
    });
    return api("/uploads/finalize", { key: init.key });
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

The presigned URL points at `AWS_S3_ENDPOINT` when `USING_NON_AWS_COMPATIBLE_STORE` is
set. That endpoint must be reachable from the browser, not only from the server.

## Failure modes

| Status | Meaning |
| --- | --- |
| 401 | No valid session cookie |
| 400 | Invalid file name/type/size, bad part number, or malformed body |
| 404 | Unknown upload (expired, never issued, or owned by another user), or the object has not landed in S3 yet |
| 409 | Multipart upload not completed yet, or finalize already in progress |
| 503 | The server has no room to stage more files; retry after uploads finish |

Upload sessions are kept in memory for 24 hours. Finalizing a finished single `PUT`
survives a server restart because the server can rebuild the session from the key and
the object in S3. An in-progress multipart upload does not: its upload ID is lost, so
the client has to start that upload again.
