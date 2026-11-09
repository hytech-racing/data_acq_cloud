package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPresignPutObjectSignsContentType(t *testing.T) {
	repository := NewS3Session("access-key", "secret-key", "us-east-1", "test-bucket", "http://localhost:9000", true)

	signed, err := repository.PresignPutObject(context.Background(), "direct-uploads/user/id/run.mcap", "application/octet-stream", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignPutObject() returned unexpected error: %v", err)
	}

	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("could not parse signed URL: %v", err)
	}

	if parsed.Path != "/test-bucket/direct-uploads/user/id/run.mcap" {
		t.Errorf("signed URL path = %q", parsed.Path)
	}
	if algorithm := parsed.Query().Get("X-Amz-Algorithm"); algorithm != "AWS4-HMAC-SHA256" {
		t.Errorf("X-Amz-Algorithm = %q", algorithm)
	}
	if expires := parsed.Query().Get("X-Amz-Expires"); expires != "300" {
		t.Errorf("X-Amz-Expires = %q, want 300", expires)
	}

	signedHeaders, err := url.QueryUnescape(parsed.Query().Get("X-Amz-SignedHeaders"))
	if err != nil {
		t.Fatalf("could not decode signed headers: %v", err)
	}
	for _, header := range strings.Split(signedHeaders, ";") {
		t.Logf("signed header: %s", header)
	}
}

func TestPresignUploadPartIncludesPartNumber(t *testing.T) {
	repository := NewS3Session("access-key", "secret-key", "us-east-1", "test-bucket", "http://localhost:9000", true)

	signed, err := repository.PresignUploadPart(context.Background(), "direct-uploads/user/id/run.mcap", "upload-id", 7, 15*time.Minute)
	if err != nil {
		t.Fatalf("PresignUploadPart() returned unexpected error: %v", err)
	}

	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("could not parse signed URL: %v", err)
	}
	if partNumber := parsed.Query().Get("partNumber"); partNumber != "7" {
		t.Errorf("partNumber = %q, want 7", partNumber)
	}
	if uploadID := parsed.Query().Get("uploadId"); uploadID != "upload-id" {
		t.Errorf("uploadId = %q, want upload-id", uploadID)
	}
}

func TestDownloadObjectRangedParts(t *testing.T) {
	data := make([]byte, 20*1024*1024)
	for i := range data {
		data[i] = byte(i)
	}
	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "run.mcap", time.Time{}, bytes.NewReader(data))
	}))
	defer server.Close()

	repository := NewS3Session("access-key", "secret-key", "us-east-1", "test-bucket", server.URL, true)

	file, err := os.Create(filepath.Join(t.TempDir(), "downloaded.mcap"))
	if err != nil {
		t.Fatalf("os.Create() error: %v", err)
	}
	defer file.Close()

	downloaded, err := repository.DownloadObject(context.Background(), "direct-uploads/run.mcap", file)
	if err != nil {
		t.Fatalf("DownloadObject() error: %v", err)
	}
	if downloaded != int64(len(data)) {
		t.Errorf("DownloadObject() = %d bytes, want %d", downloaded, len(data))
	}

	if got := sha256File(t, file); got != want {
		t.Errorf("downloaded file SHA-256 = %q, want %q", got, want)
	}
}

func sha256File(t *testing.T, file *os.File) string {
	t.Helper()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek() error: %v", err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		t.Fatalf("io.Copy() error: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestDownloadObjectNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message><Key>missing.mcap</Key></Error>`)
	}))
	defer server.Close()

	repository := NewS3Session("access-key", "secret-key", "us-east-1", "test-bucket", server.URL, true)

	file, err := os.Create(filepath.Join(t.TempDir(), "downloaded.mcap"))
	if err != nil {
		t.Fatalf("os.Create() error: %v", err)
	}
	defer file.Close()

	if _, err := repository.DownloadObject(context.Background(), "missing.mcap", file); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("DownloadObject() error = %v, want ErrObjectNotFound", err)
	}
}
