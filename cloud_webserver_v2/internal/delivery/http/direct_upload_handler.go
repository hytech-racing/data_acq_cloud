package http

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/render"
	"github.com/hytech-racing/cloud-webserver-v2/internal/background"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
)

// Direct uploads keep file bytes away from this server. The client asks for signed
// URLs, uploads to S3 itself, then asks this server to process the finished object.
const (
	// At 64 MiB the 20 GiB file cap yields at most 320 parts, far below S3's limit
	// of 10,000, and every part except the last clears S3's 5 MiB minimum.
	uploadPartSize int64 = 64 << 20

	// Part URLs are issued at initiate, so they must stay valid for the whole
	// transfer and expire with the session.
	directUploadTTL    = 24 * time.Hour
	directUploadPrefix = "direct-uploads"

	// 16 random bytes encoded as unpadded base64url.
	directUploadIDLength = 22

	maxJSONBodySize = 1 << 20
)

var (
	errUploadNotFound   = errors.New("upload not found")
	errUploadFinalizing = errors.New("upload is already being finalized")
)

type presignUploadRequest struct {
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

type completedPartRequest struct {
	PartNumber int32  `json:"partNumber"`
	ETag       string `json:"eTag"`
}

type abortUploadRequest struct {
	Key      string `json:"key"`
	UploadID string `json:"uploadId"`
}

type finalizeUploadRequest struct {
	Key      string                 `json:"key"`
	UploadID string                 `json:"uploadId"`
	Parts    []completedPartRequest `json:"parts"`
	Hidden   bool                   `json:"hidden"`
}

type uploadPartURL struct {
	PartNumber int32  `json:"partNumber"`
	URL        string `json:"url"`
}

// directUploadSession records an upload the server authorized. A client cannot invent a
// key, finalize an unknown key, or complete another user's multipart upload.
type directUploadSession struct {
	Key        string
	UploadID   string
	Owner      string
	FileName   string
	Size       int64
	Finalizing bool
	JobID      string
	ExpiresAt  time.Time
}

type directUploadRegistry struct {
	mu       sync.Mutex
	sessions map[string]*directUploadSession
}

func newDirectUploadRegistry() *directUploadRegistry {
	return &directUploadRegistry{sessions: make(map[string]*directUploadSession)}
}

func (registry *directUploadRegistry) add(session *directUploadSession) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.sessions[session.Key] = session
}

// session returns a live session owned by owner. The caller must hold mu.
func (registry *directUploadRegistry) session(key, owner string) (*directUploadSession, error) {
	session, ok := registry.sessions[key]
	if !ok || session.Owner != owner || time.Now().After(session.ExpiresAt) {
		return nil, errUploadNotFound
	}
	return session, nil
}

// claimFinalize reserves the upload for finalization. A retried finalize gets the
// existing job ID, so it never processes the same file twice.
func (registry *directUploadRegistry) claimFinalize(key, owner string) (directUploadSession, string, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	session, err := registry.session(key, owner)
	if err != nil {
		return directUploadSession{}, "", err
	}
	if session.JobID != "" {
		return *session, session.JobID, nil
	}
	if session.Finalizing {
		return *session, "", errUploadFinalizing
	}
	session.Finalizing = true
	return *session, "", nil
}

// take removes and returns a session so the caller can abort its S3 state.
func (registry *directUploadRegistry) take(key, owner string) (directUploadSession, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	session, err := registry.session(key, owner)
	if err != nil {
		return directUploadSession{}, err
	}
	delete(registry.sessions, key)
	return *session, nil
}

func (registry *directUploadRegistry) takeExpired(now time.Time) []directUploadSession {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	expired := make([]directUploadSession, 0)
	for key, session := range registry.sessions {
		if now.After(session.ExpiresAt) {
			expired = append(expired, *session)
			delete(registry.sessions, key)
		}
	}

	return expired
}

func (h *uploadHandler) InitiateMultipartUpload(w http.ResponseWriter, r *http.Request) *HandlerError {
	var request presignUploadRequest
	return h.handleJSON(w, r, &request, func(owner string) (any, *HandlerError) {
		h.pruneExpiredUploads(r.Context())

		fileName, handlerErr := sanitizeUploadFileName(request.FileName)
		if handlerErr != nil {
			return nil, handlerErr
		}
		if handlerErr := h.validateUploadSize(request.Size); handlerErr != nil {
			return nil, handlerErr
		}
		contentType := strings.TrimSpace(request.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		id := make([]byte, 16)
		rand.Read(id)
		key := path.Join(directUploadPrefix, owner, base64.RawURLEncoding.EncodeToString(id), fileName)
		uploadID, err := h.s3Repository.CreateMultipartUpload(r.Context(), key, contentType)
		if err != nil {
			log.Printf("failed to create multipart upload: %v", err)
			return nil, NewHandlerError("failed to prepare upload", http.StatusInternalServerError)
		}

		count := int32((request.Size + uploadPartSize - 1) / uploadPartSize)
		parts := make([]uploadPartURL, 0, count)
		for partNumber := int32(1); partNumber <= count; partNumber++ {
			partURL, err := h.s3Repository.PresignUploadPart(r.Context(), key, uploadID, partNumber, directUploadTTL)
			if err != nil {
				log.Printf("failed to presign part %d: %v", partNumber, err)
				if abortErr := h.s3Repository.AbortMultipartUpload(r.Context(), key, uploadID); abortErr != nil {
					log.Printf("failed to abort multipart upload: %v", abortErr)
				}
				return nil, NewHandlerError("failed to prepare upload", http.StatusInternalServerError)
			}
			parts = append(parts, uploadPartURL{PartNumber: partNumber, URL: partURL})
		}

		h.directUploads.add(&directUploadSession{
			Key:       key,
			UploadID:  uploadID,
			Owner:     owner,
			FileName:  fileName,
			Size:      request.Size,
			ExpiresAt: time.Now().Add(directUploadTTL),
		})

		w.Header().Set("Cache-Control", "no-store")
		return map[string]any{
			"uploadId": uploadID,
			"key":      key,
			"partSize": uploadPartSize,
			"parts":    parts,
		}, nil
	})
}

func (h *uploadHandler) AbortMultipartUpload(w http.ResponseWriter, r *http.Request) *HandlerError {
	var request abortUploadRequest
	return h.handleJSON(w, r, &request, func(owner string) (any, *HandlerError) {
		session, err := h.directUploads.take(request.Key, owner)
		if err != nil {
			return nil, NewHandlerError("unknown upload", http.StatusNotFound)
		}
		if err := h.s3Repository.AbortMultipartUpload(r.Context(), session.Key, session.UploadID); err != nil {
			log.Printf("failed to abort multipart upload: %v", err)
			return nil, NewHandlerError("failed to abort upload", http.StatusInternalServerError)
		}
		return map[string]any{}, nil
	})
}

// FinalizeUpload confirms that a direct upload finished and queues the MCAP for the
// same processing pipeline the multipart form upload uses. A multipart client sends
// its upload ID and parts here so the server assembles, checks, and queues in one call.
func (h *uploadHandler) FinalizeUpload(w http.ResponseWriter, r *http.Request) *HandlerError {
	var request finalizeUploadRequest
	return h.handleJSON(w, r, &request, func(owner string) (any, *HandlerError) {
		h.pruneExpiredUploads(r.Context())

		if request.UploadID == "" {
			return nil, NewHandlerError("uploadId is required", http.StatusBadRequest)
		}

		session, existingJobID, err := h.directUploads.claimFinalize(request.Key, owner)
		if errors.Is(err, errUploadFinalizing) {
			return nil, NewHandlerError("upload is already being finalized", http.StatusConflict)
		}
		live := err == nil
		if !live {
			// Sessions live in memory, so rebuild one from the key after a restart. The
			// key is server-generated and scoped to this user; S3 holds the object.
			recovered, ok := recoverDirectUploadSession(request.Key, owner)
			if !ok {
				return nil, NewHandlerError("unknown upload", http.StatusNotFound)
			}
			session = recovered
		}
		if existingJobID != "" {
			return map[string]any{"jobId": existingJobID}, nil
		}

		jobID := ""
		// Reset the in-progress marker on failure; on success the job ID makes a retry a no-op.
		defer func() {
			h.directUploads.mu.Lock()
			if session, ok := h.directUploads.sessions[request.Key]; ok {
				session.Finalizing = false
				session.JobID = jobID
			}
			h.directUploads.mu.Unlock()
		}()

		if live && request.UploadID != session.UploadID {
			return nil, NewHandlerError("upload id does not match the upload", http.StatusBadRequest)
		}

		size, exists, handlerErr := h.uploadedSize(r.Context(), session.Key)
		if handlerErr != nil {
			return nil, handlerErr
		}
		if !exists {
			if !live {
				return nil, NewHandlerError("upload session expired; restart the multipart upload", http.StatusConflict)
			}
			parts := make([]s3.MultipartPart, 0, len(request.Parts))
			for _, part := range request.Parts {
				parts = append(parts, s3.MultipartPart{PartNumber: part.PartNumber, ETag: part.ETag})
			}
			if err := h.s3Repository.CompleteMultipartUpload(r.Context(), session.Key, session.UploadID, parts); err != nil {
				log.Printf("failed to complete multipart upload: %v", err)
				return nil, NewHandlerError("failed to complete upload", http.StatusInternalServerError)
			}
			size, exists, handlerErr = h.uploadedSize(r.Context(), session.Key)
			if handlerErr != nil {
				return nil, handlerErr
			}
			if !exists {
				log.Printf("direct upload %v is missing after completing multipart upload", session.Key)
				return nil, NewHandlerError("failed to inspect uploaded file", http.StatusInternalServerError)
			}
		}
		if session.Size > 0 && size != session.Size {
			log.Printf("direct upload %v declared %d bytes but stored %d bytes", session.Key, session.Size, size)
		}
		if handlerErr := h.validateUploadSize(size); handlerErr != nil {
			// The client broke the size contract, so do not leave the object in the bucket.
			if deleteErr := h.s3Repository.DeleteObject(r.Context(), h.s3Repository.Bucket(), session.Key); deleteErr != nil {
				log.Printf("failed to delete rejected direct upload %v: %v", session.Key, deleteErr)
			}
			return nil, handlerErr
		}
		if !h.fileProcessor.HasCapacityFor(size) {
			return nil, NewHandlerError("server is out of upload capacity, try again later", http.StatusServiceUnavailable)
		}

		job, err := h.fileProcessor.EnqueueS3Object(
			session.Key,
			session.FileName,
			size,
			&background.PostProcessMCAPUploadJob{},
			request.Hidden,
		)
		if err != nil {
			log.Printf("failed to enqueue direct upload %v: %v", session.Key, err)
			return nil, NewHandlerError("failed to queue file for processing", http.StatusInternalServerError)
		}

		jobID = job.ID
		return map[string]any{"jobId": job.ID}, nil
	})
}

func (h *uploadHandler) uploadedSize(ctx context.Context, key string) (int64, bool, *HandlerError) {
	size, err := h.s3Repository.HeadObject(ctx, key)
	if errors.Is(err, s3.ErrObjectNotFound) {
		return 0, false, nil
	}
	if err != nil {
		log.Printf("failed to inspect direct upload %v: %v", key, err)
		return 0, false, NewHandlerError("failed to inspect uploaded file", http.StatusInternalServerError)
	}
	return size, true, nil
}

func (h *uploadHandler) validateUploadSize(size int64) *HandlerError {
	if size <= 0 {
		return NewHandlerError("size must be a positive number of bytes", http.StatusBadRequest)
	}
	if size > h.fileProcessor.MaxTotalSize() {
		return NewHandlerError("file is larger than the server's maximum file size", http.StatusBadRequest)
	}
	return nil
}

// pruneExpiredUploads aborts expired multipart uploads so their parts stop costing storage.
func (h *uploadHandler) pruneExpiredUploads(ctx context.Context) {
	for _, session := range h.directUploads.takeExpired(time.Now()) {
		if err := h.s3Repository.AbortMultipartUpload(ctx, session.Key, session.UploadID); err != nil {
			log.Printf("failed to abort expired multipart upload %v: %v", session.Key, err)
		}
	}
}

// handleJSON runs the auth, decode, error, and render steps every direct upload
// endpoint shares, leaving the callback with only its own request logic.
func (h *uploadHandler) handleJSON(w http.ResponseWriter, r *http.Request, target any, handle func(owner string) (any, *HandlerError)) *HandlerError {
	session, err := h.authHandler.CurrentSession(r)
	if err != nil {
		return NewHandlerError("not authenticated", http.StatusUnauthorized)
	}
	if strings.ContainsAny(session.User.Username, "./") {
		return NewHandlerError("are you sure this is a GT username?", http.StatusUnauthorized)
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBodySize)).Decode(target); err != nil {
		return NewHandlerError("invalid request body", http.StatusBadRequest)
	}

	data, handlerErr := handle(session.User.Username)
	if handlerErr != nil {
		return handlerErr
	}
	render.JSON(w, r, data)
	return nil
}

func sanitizeUploadFileName(raw string) (string, *HandlerError) {
	fileName := strings.TrimSpace(raw)
	switch {
	case fileName == "":
		return "", NewHandlerError("fileName is required", http.StatusBadRequest)
	case len(fileName) > 255:
		return "", NewHandlerError("fileName is too long", http.StatusBadRequest)
	case strings.ContainsAny(fileName, `/\`):
		return "", NewHandlerError("fileName must not contain path separators", http.StatusBadRequest)
	case strings.ContainsFunc(fileName, func(character rune) bool { return character < 0x20 || character == 0x7f }):
		return "", NewHandlerError("fileName contains invalid characters", http.StatusBadRequest)
	case !strings.EqualFold(path.Ext(fileName), ".mcap"):
		return "", NewHandlerError("only .mcap files can be uploaded", http.StatusBadRequest)
	}
	return fileName, nil
}

func recoverDirectUploadSession(key, owner string) (directUploadSession, bool) {
	rest, ok := strings.CutPrefix(key, path.Join(directUploadPrefix, owner)+"/")
	if !ok {
		return directUploadSession{}, false
	}
	id, rawFileName, ok := strings.Cut(rest, "/")
	if !ok || len(id) != directUploadIDLength {
		return directUploadSession{}, false
	}
	fileName, handlerErr := sanitizeUploadFileName(rawFileName)
	if handlerErr != nil {
		return directUploadSession{}, false
	}
	return directUploadSession{Key: key, Owner: owner, FileName: fileName}, true
}
