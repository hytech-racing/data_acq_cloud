package hytech_middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hytech-racing/cloud-webserver-v2/internal/background"
	"github.com/stretchr/testify/require"
)

type atomicInt64 struct {
	val int64
}

func (a *atomicInt64) Load() int64 {
	return atomic.LoadInt64(&a.val)
}

func (a *atomicInt64) Add(n int64) {
	atomic.AddInt64(&a.val, n)
}

func newTestFileProcessor(t *testing.T, dir string, limit int64, current int64) *background.FileProcessor {
	fp, err := background.NewFileProcessor(dir, limit, nil, nil)
	require.NoError(t, err)
	fp.MiddlewareEstimatedSize.Store(current)
	return fp
}

type dummyHandler struct {
	called *bool
}

func (d *dummyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	*d.called = true
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Success"))
}

func TestFileUploadSizeLimitMiddleware_ValidUpload(t *testing.T) {
	fp := newTestFileProcessor(t, "/tmp/valid_upload", 1000, 100)
	middleware := &FileUploadMiddleware{FileProcessor: fp}

	called := false
	handler := middleware.FileUploadSizeLimitMiddleware(&dummyHandler{called: &called})

	req := httptest.NewRequest("POST", "/upload", io.NopCloser(bytes.NewReader(make([]byte, 200))))
	req.ContentLength = 200
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if !called {
		t.Errorf("Expected handler to be called, but it wasn't")
	}

	if resp.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d", resp.Code)
	}
}

func TestFileUploadSizeLimitMiddleware_MissingContentLength(t *testing.T) {
	fp := newTestFileProcessor(t, "/tmp/missing_content_length", 1000, 100)
	middleware := &FileUploadMiddleware{FileProcessor: fp}

	handler := middleware.FileUploadSizeLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Handler should not be called if Content-Length is missing")
	}))

	req := httptest.NewRequest("POST", "/upload", nil)
	req.ContentLength = -1
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 Bad Request, got %d", resp.Code)
	}
}

func TestFileUploadSizeLimitMiddleware_ExceedsLimit(t *testing.T) {
	fp := newTestFileProcessor(t, "/tmp/exceeds_limit/", 1000, 950)
	middleware := &FileUploadMiddleware{FileProcessor: fp}

	handler := middleware.FileUploadSizeLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Handler should not be called when upload exceeds size limit")
	}))

	req := httptest.NewRequest("POST", "/upload", io.NopCloser(bytes.NewReader(make([]byte, 100))))
	req.ContentLength = 100
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 Service Unavailable, got %d", resp.Code)
	}
}
