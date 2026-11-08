package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hytech-racing/cloud-webserver-v2/internal/auth"
	"github.com/hytech-racing/cloud-webserver-v2/internal/background"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
)

const contractCookieName = "contract_session"

type uploadContract struct {
	handler *uploadHandler
	cookie  *http.Cookie
	s3      *httptest.Server
}

func newUploadContract(t *testing.T) *uploadContract {
	t.Helper()

	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case r.Method == http.MethodPost && r.URL.Query().Has("uploads"):
			io.WriteString(w, `<InitiateMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>bucket</Bucket><Key>key</Key><UploadId>upload-id</UploadId></InitiateMultipartUploadResult>`)
		case r.Method == http.MethodPost && r.URL.Query().Get("uploadId") != "":
			io.WriteString(w, `<CompleteMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>bucket</Bucket><Key>key</Key><ETag>"etag"</ETag></CompleteMultipartUploadResult>`)
		case r.Method == http.MethodHead:
			w.Header().Set("Content-Length", "1024")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(s3Server.Close)

	repository := s3.NewS3Session("access-key", "secret-key", "us-east-1", "bucket", s3Server.URL, true)
	processor, err := background.NewFileProcessor(t.TempDir(), 20<<30, nil, repository)
	if err != nil {
		t.Fatalf("could not create file processor: %v", err)
	}

	sessions := auth.NewMemorySessionStore(time.Hour)
	t.Cleanup(sessions.Close)
	session, err := sessions.Create(&auth.User{Username: "user"})
	if err != nil {
		t.Fatalf("could not create session: %v", err)
	}

	return &uploadContract{
		handler: &uploadHandler{
			fileProcessor: processor,
			s3Repository:  repository,
			authHandler: &AuthHandler{
				config:   auth.AuthConfig{CookieName: contractCookieName},
				sessions: sessions,
			},
			directUploads: newDirectUploadRegistry(),
		},
		cookie: &http.Cookie{Name: contractCookieName, Value: session.ID},
		s3:     s3Server,
	}
}

func (contract *uploadContract) post(t *testing.T, handler HandlerFunc, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	return contract.send(t, handler, path, body, true)
}

func (contract *uploadContract) postAnonymously(t *testing.T, handler HandlerFunc, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	return contract.send(t, handler, path, body, false)
}

func (contract *uploadContract) send(t *testing.T, handler HandlerFunc, path string, body any, authenticated bool) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()

	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatalf("could not encode request body: %v", err)
		}
	}

	request := httptest.NewRequest(http.MethodPost, path, &payload)
	request.Header.Set("Content-Type", "application/json")
	if authenticated {
		request.AddCookie(contract.cookie)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	decoded := make(map[string]any)
	if recorder.Body.Len() > 0 {
		if err := json.NewDecoder(recorder.Body).Decode(&decoded); err != nil {
			t.Fatalf("could not decode response body %q: %v", recorder.Body.String(), err)
		}
	}
	return recorder, decoded
}

func responseData(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("response data = %#v, want an object", body["data"])
	}
	return data
}

func TestDirectUploadContract(t *testing.T) {
	contract := newUploadContract(t)
	presign := HandlerFunc(contract.handler.PresignUpload)
	initiate := HandlerFunc(contract.handler.InitiateMultipartUpload)
	parts := HandlerFunc(contract.handler.PresignUploadParts)
	complete := HandlerFunc(contract.handler.CompleteMultipartUpload)
	abort := HandlerFunc(contract.handler.AbortMultipartUpload)
	finalize := HandlerFunc(contract.handler.FinalizeUpload)

	t.Run("unauthenticated requests are rejected", func(t *testing.T) {
		recorder, body := contract.postAnonymously(t, presign, "/uploads/presign", map[string]any{"fileName": "run.mcap", "size": 1024})
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
		}
		if body["message"] != "not authenticated" {
			t.Errorf("message = %#v, want %q", body["message"], "not authenticated")
		}
	})

	t.Run("presign validates the request", func(t *testing.T) {
		tests := []struct {
			name    string
			body    map[string]any
			message string
		}{
			{"missing name", map[string]any{"size": 1024}, "fileName is required"},
			{"wrong type", map[string]any{"fileName": "run.h5", "size": 1024}, "only .mcap files can be uploaded"},
			{"zero size", map[string]any{"fileName": "run.mcap", "size": 0}, "size must be a positive number of bytes"},
			{"too large for one put", map[string]any{"fileName": "run.mcap", "size": maxSinglePutSize + 1}, "files larger than 5368709120 bytes must use the multipart upload endpoints"},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				recorder, body := contract.post(t, presign, "/uploads/presign", test.body)
				if recorder.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
				}
				if body["message"] != test.message {
					t.Errorf("message = %#v, want %q", body["message"], test.message)
				}
			})
		}
	})

	var presignedKey string
	t.Run("presign returns a scoped key and url", func(t *testing.T) {
		recorder, body := contract.post(t, presign, "/uploads/presign", map[string]any{
			"fileName":    " run.mcap ",
			"contentType": "application/mcap",
			"size":        1024,
		})
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %#v)", recorder.Code, http.StatusOK, body)
		}
		if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("Cache-Control = %q, want %q", got, "no-store")
		}

		data := responseData(t, body)
		presignedKey, _ = data["key"].(string)
		if !strings.HasPrefix(presignedKey, "direct-uploads/user/") || !strings.HasSuffix(presignedKey, "/run.mcap") {
			t.Errorf("key = %q, want a key scoped to the user and file", presignedKey)
		}
		if data["expiresIn"] != float64(300) {
			t.Errorf("expiresIn = %#v, want 300", data["expiresIn"])
		}

		parsed, err := url.Parse(data["uploadUrl"].(string))
		if err != nil {
			t.Fatalf("could not parse uploadUrl: %v", err)
		}
		if parsed.Host != strings.TrimPrefix(contract.s3.URL, "http://") {
			t.Errorf("uploadUrl host = %q, want %q", parsed.Host, strings.TrimPrefix(contract.s3.URL, "http://"))
		}
	})

	var multipartKey, uploadID string
	t.Run("initiate creates a multipart upload", func(t *testing.T) {
		recorder, body := contract.post(t, initiate, "/uploads/initiate", map[string]any{
			"fileName":    "run.mcap",
			"contentType": "application/mcap",
			"size":        1024,
		})
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %#v)", recorder.Code, http.StatusOK, body)
		}

		data := responseData(t, body)
		multipartKey, _ = data["key"].(string)
		uploadID, _ = data["uploadId"].(string)
		if uploadID != "upload-id" {
			t.Errorf("uploadId = %q, want %q", uploadID, "upload-id")
		}
	})

	t.Run("parts are signed in ascending order", func(t *testing.T) {
		recorder, body := contract.post(t, parts, "/uploads/parts", map[string]any{
			"key":         multipartKey,
			"uploadId":    uploadID,
			"partNumbers": []int32{3, 1},
		})
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %#v)", recorder.Code, http.StatusOK, body)
		}

		signed := responseData(t, body)["parts"].([]any)
		if len(signed) != 2 {
			t.Fatalf("len(parts) = %d, want 2", len(signed))
		}
		for index, want := range []float64{1, 3} {
			part := signed[index].(map[string]any)
			if part["partNumber"] != want {
				t.Errorf("parts[%d].partNumber = %#v, want %v", index, part["partNumber"], want)
			}
			if !strings.Contains(part["url"].(string), "partNumber=") {
				t.Errorf("parts[%d].url does not carry a part number", index)
			}
		}
	})

	t.Run("complete is idempotent", func(t *testing.T) {
		request := map[string]any{
			"key":      multipartKey,
			"uploadId": uploadID,
			"parts": []map[string]any{
				{"partNumber": 2, "eTag": "etag-2"},
				{"partNumber": 1, "eTag": `"etag-1"`},
			},
		}

		for attempt := 0; attempt < 2; attempt++ {
			recorder, body := contract.post(t, complete, "/uploads/complete", request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("attempt %d status = %d, want %d (body %#v)", attempt, recorder.Code, http.StatusOK, body)
			}
			if responseData(t, body)["key"] != multipartKey {
				t.Errorf("attempt %d key = %#v, want %q", attempt, responseData(t, body)["key"], multipartKey)
			}
		}
	})

	t.Run("finalize enqueues exactly one job", func(t *testing.T) {
		recorder, body := contract.post(t, finalize, "/uploads/finalize", map[string]any{"key": multipartKey})
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %#v)", recorder.Code, http.StatusOK, body)
		}
		jobID, _ := responseData(t, body)["jobId"].(string)
		if jobID == "" {
			t.Fatalf("jobId is empty")
		}

		_, retryBody := contract.post(t, finalize, "/uploads/finalize", map[string]any{"key": multipartKey})
		if retryJobID := responseData(t, retryBody)["jobId"]; retryJobID != jobID {
			t.Errorf("retry jobId = %#v, want %q", retryJobID, jobID)
		}
	})

	t.Run("finalize accepts a single put", func(t *testing.T) {
		recorder, body := contract.post(t, finalize, "/uploads/finalize", map[string]any{"key": presignedKey})
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %#v)", recorder.Code, http.StatusOK, body)
		}
		if responseData(t, body)["jobId"] == "" {
			t.Errorf("jobId is empty")
		}
	})

	t.Run("multipart-only requests reject a single put key", func(t *testing.T) {
		recorder, body := contract.post(t, complete, "/uploads/complete", map[string]any{
			"key":      presignedKey,
			"uploadId": "upload-id",
			"parts":    []map[string]any{{"partNumber": 1, "eTag": "etag"}},
		})
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
		}
		if body["message"] != "upload is not a multipart upload" {
			t.Errorf("message = %#v", body["message"])
		}
	})

	t.Run("abort rejects an unknown upload", func(t *testing.T) {
		recorder, body := contract.post(t, abort, "/uploads/abort", map[string]any{
			"key":      "direct-uploads/user/abcdefghijklmnopqrstuv/run.mcap",
			"uploadId": "missing",
		})
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
		}
		if body["message"] != "unknown upload" {
			t.Errorf("message = %#v", body["message"])
		}
	})
}
