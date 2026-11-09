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
	// S3 rejects a single PUT larger than 5 GiB; larger files use the multipart endpoints.
	maxSinglePutSize = int64(5 * 1024 * 1024 * 1024)

	maxPartNumber = 10000

	// Signing thousands of URLs at once burns CPU, so clients ask in batches.
	maxPresignedPartsPerRequest = 100

	// Signed URL lifetimes. A transfer may outlive the window once it has started.
	presignExpiry      = 5 * time.Minute
	uploadPartExpiry   = 15 * time.Minute
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

type presignPartsRequest struct {
	Key         string  `json:"key"`
	UploadID    string  `json:"uploadId"`
	PartNumbers []int32 `json:"partNumbers"`
}

type completedPartRequest struct {
	PartNumber int32  `json:"partNumber"`
	ETag       string `json:"eTag"`
}

type completeUploadRequest struct {
	Key      string                 `json:"key"`
	UploadID string                 `json:"uploadId"`
	Parts    []completedPartRequest `json:"parts"`
}

type abortUploadRequest struct {
	Key      string `json:"key"`
	UploadID string `json:"uploadId"`
}

type finalizeUploadRequest struct {
	Key    string `json:"key"`
	Hidden bool   `json:"hidden"`
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
	Multipart  bool
	Completed  bool
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

// lookup returns a copy so handlers can talk to S3 without holding the lock.
func (registry *directUploadRegistry) lookup(key, owner string) (directUploadSession, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	session, err := registry.session(key, owner)
	if err != nil {
		return directUploadSession{}, err
	}
	return *session, nil
}

func (registry *directUploadRegistry) markCompleted(key, owner, uploadID string) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	session, err := registry.session(key, owner)
	if err != nil {
		return err
	}
	if session.UploadID != uploadID {
		return errUploadNotFound
	}
	session.Completed = true
	return nil
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

// finishFinalize clears the in-progress marker and records the job on success. A
// retried finalize then returns the same job instead of processing the file twice.
func (registry *directUploadRegistry) finishFinalize(key, jobID string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if session, ok := registry.sessions[key]; ok {
		session.Finalizing = false
		session.JobID = jobID
	}
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

// PresignUpload signs a single PUT so the client can send a file straight to S3.
func (h *uploadHandler) PresignUpload(w http.ResponseWriter, r *http.Request) *HandlerError {
	var request presignUploadRequest
	return h.handleJSON(w, r, &request, func(owner string) (any, *HandlerError) {
		h.pruneExpiredUploads(r.Context())

		fileName, contentType, handlerErr := h.validateStart(request, maxSinglePutSize)
		if handlerErr != nil {
			return nil, handlerErr
		}

		key, handlerErr := h.newUploadKey(owner, fileName)
		if handlerErr != nil {
			return nil, handlerErr
		}
		uploadURL, err := h.s3Repository.PresignPutObject(r.Context(), key, contentType, presignExpiry)
		if err != nil {
			log.Printf("failed to presign direct upload: %v", err)
			return nil, NewHandlerError("failed to prepare upload", http.StatusInternalServerError)
		}

		h.directUploads.add(&directUploadSession{
			Key:       key,
			Owner:     owner,
			FileName:  fileName,
			Size:      request.Size,
			ExpiresAt: time.Now().Add(directUploadTTL),
		})

		w.Header().Set("Cache-Control", "no-store")
		return map[string]any{
			"uploadUrl": uploadURL,
			"key":       key,
			"expiresIn": int(presignExpiry.Seconds()),
		}, nil
	})
}

// InitiateMultipartUpload starts a multipart upload for a file too large or too
// unreliable for a single PUT.
func (h *uploadHandler) InitiateMultipartUpload(w http.ResponseWriter, r *http.Request) *HandlerError {
	var request presignUploadRequest
	return h.handleJSON(w, r, &request, func(owner string) (any, *HandlerError) {
		h.pruneExpiredUploads(r.Context())

		fileName, contentType, handlerErr := h.validateStart(request, 0)
		if handlerErr != nil {
			return nil, handlerErr
		}

		key, handlerErr := h.newUploadKey(owner, fileName)
		if handlerErr != nil {
			return nil, handlerErr
		}
		uploadID, err := h.s3Repository.CreateMultipartUpload(r.Context(), key, contentType)
		if err != nil {
			log.Printf("failed to create multipart upload: %v", err)
			return nil, NewHandlerError("failed to prepare upload", http.StatusInternalServerError)
		}

		h.directUploads.add(&directUploadSession{
			Key:       key,
			UploadID:  uploadID,
			Owner:     owner,
			FileName:  fileName,
			Size:      request.Size,
			Multipart: true,
			ExpiresAt: time.Now().Add(directUploadTTL),
		})

		w.Header().Set("Cache-Control", "no-store")
		return map[string]any{"uploadId": uploadID, "key": key}, nil
	})
}

// PresignUploadParts signs a batch of part URLs for an in-progress multipart upload.
func (h *uploadHandler) PresignUploadParts(w http.ResponseWriter, r *http.Request) *HandlerError {
	var request presignPartsRequest
	return h.handleJSON(w, r, &request, func(owner string) (any, *HandlerError) {
		h.pruneExpiredUploads(r.Context())

		partNumbers, handlerErr := validatePartNumbers(request.PartNumbers, maxPresignedPartsPerRequest)
		if handlerErr != nil {
			return nil, handlerErr
		}
		session, handlerErr := h.multipartSession(request.Key, owner, request.UploadID)
		if handlerErr != nil {
			return nil, handlerErr
		}

		parts := make([]uploadPartURL, 0, len(partNumbers))
		for _, partNumber := range partNumbers {
			partURL, err := h.s3Repository.PresignUploadPart(r.Context(), session.Key, session.UploadID, partNumber, uploadPartExpiry)
			if err != nil {
				log.Printf("failed to presign part %d: %v", partNumber, err)
				return nil, NewHandlerError("failed to prepare upload", http.StatusInternalServerError)
			}
			parts = append(parts, uploadPartURL{PartNumber: partNumber, URL: partURL})
		}

		w.Header().Set("Cache-Control", "no-store")
		return map[string]any{"parts": parts}, nil
	})
}

// CompleteMultipartUpload asks S3 to assemble the parts the client uploaded.
func (h *uploadHandler) CompleteMultipartUpload(w http.ResponseWriter, r *http.Request) *HandlerError {
	var request completeUploadRequest
	return h.handleJSON(w, r, &request, func(owner string) (any, *HandlerError) {
		session, handlerErr := h.multipartSession(request.Key, owner, request.UploadID)
		if handlerErr != nil {
			return nil, handlerErr
		}
		// S3 rejects an upload ID that already finished, so a retried complete is a no-op.
		if session.Completed {
			return map[string]any{"key": session.Key}, nil
		}
		if len(request.Parts) == 0 {
			return nil, NewHandlerError("parts must not be empty", http.StatusBadRequest)
		}
		if len(request.Parts) > maxPartNumber {
			return nil, NewHandlerError(fmt.Sprintf("at most %d parts are allowed", maxPartNumber), http.StatusBadRequest)
		}

		parts := make([]s3.MultipartPart, 0, len(request.Parts))
		seen := make(map[int32]struct{}, len(request.Parts))
		for _, part := range request.Parts {
			if part.PartNumber < 1 || part.PartNumber > maxPartNumber {
				return nil, NewHandlerError(fmt.Sprintf("part numbers must be between 1 and %d", maxPartNumber), http.StatusBadRequest)
			}
			if _, ok := seen[part.PartNumber]; ok {
				return nil, NewHandlerError(fmt.Sprintf("duplicate part number %d", part.PartNumber), http.StatusBadRequest)
			}
			eTag, err := normalizeETag(part.ETag)
			if err != nil {
				return nil, NewHandlerError(err.Error(), http.StatusBadRequest)
			}
			seen[part.PartNumber] = struct{}{}
			parts = append(parts, s3.MultipartPart{PartNumber: part.PartNumber, ETag: eTag})
		}
		sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })

		if err := h.s3Repository.CompleteMultipartUpload(r.Context(), session.Key, session.UploadID, parts); err != nil {
			log.Printf("failed to complete multipart upload: %v", err)
			return nil, NewHandlerError("failed to complete upload", http.StatusInternalServerError)
		}
		if err := h.directUploads.markCompleted(request.Key, owner, request.UploadID); err != nil {
			return nil, NewHandlerError("unknown upload", http.StatusNotFound)
		}
		return map[string]any{"key": session.Key}, nil
	})
}

// AbortMultipartUpload discards an unfinished multipart upload.
func (h *uploadHandler) AbortMultipartUpload(w http.ResponseWriter, r *http.Request) *HandlerError {
	var request abortUploadRequest
	return h.handleJSON(w, r, &request, func(owner string) (any, *HandlerError) {
		session, err := h.directUploads.take(request.Key, owner)
		if err != nil {
			return nil, NewHandlerError("unknown upload", http.StatusNotFound)
		}
		if session.Multipart && !session.Completed && session.UploadID != "" {
			if err := h.s3Repository.AbortMultipartUpload(r.Context(), session.Key, session.UploadID); err != nil {
				log.Printf("failed to abort multipart upload: %v", err)
				return nil, NewHandlerError("failed to abort upload", http.StatusInternalServerError)
			}
		}
		return map[string]any{}, nil
	})
}

// FinalizeUpload confirms that a direct upload finished and queues the MCAP for the
// same processing pipeline the multipart form upload uses.
func (h *uploadHandler) FinalizeUpload(w http.ResponseWriter, r *http.Request) *HandlerError {
	var request finalizeUploadRequest
	return h.handleJSON(w, r, &request, func(owner string) (any, *HandlerError) {
		h.pruneExpiredUploads(r.Context())

		session, existingJobID, err := h.directUploads.claimFinalize(request.Key, owner)
		if errors.Is(err, errUploadFinalizing) {
			return nil, NewHandlerError("upload is already being finalized", http.StatusConflict)
		}
		if err != nil {
			// Sessions live in memory, so rebuild one from the key after a restart. The
			// key is server-generated and scoped to this user; S3 holds the object.
			recovered, ok := recoverDirectUploadSession(request.Key, owner)
			if !ok {
				return nil, NewHandlerError("unknown upload", http.StatusNotFound)
			}
			session, existingJobID = recovered, ""
		}
		if existingJobID != "" {
			return map[string]any{"jobId": existingJobID}, nil
		}

		fail := func(handlerErr *HandlerError) (any, *HandlerError) {
			h.directUploads.finishFinalize(request.Key, "")
			return nil, handlerErr
		}

		if session.Multipart && !session.Completed {
			return fail(NewHandlerError("multipart upload has not been completed", http.StatusConflict))
		}

		size, err := h.s3Repository.HeadObject(r.Context(), session.Key)
		if errors.Is(err, s3.ErrObjectNotFound) {
			return fail(NewHandlerError("uploaded file was not found in the bucket", http.StatusNotFound))
		}
		if err != nil {
			log.Printf("failed to inspect direct upload %v: %v", session.Key, err)
			return fail(NewHandlerError("failed to inspect uploaded file", http.StatusInternalServerError))
		}
		if session.Size > 0 && size != session.Size {
			log.Printf("direct upload %v declared %d bytes but stored %d bytes", session.Key, session.Size, size)
		}
		if handlerErr := h.validateUploadSize(size); handlerErr != nil {
			// The client broke the size contract, so do not leave the object in the bucket.
			if deleteErr := h.s3Repository.DeleteObject(r.Context(), h.s3Repository.Bucket(), session.Key); deleteErr != nil {
				log.Printf("failed to delete rejected direct upload %v: %v", session.Key, deleteErr)
			}
			return fail(handlerErr)
		}
		if !h.fileProcessor.HasCapacityFor(size) {
			return fail(NewHandlerError("server is out of upload capacity, try again later", http.StatusServiceUnavailable))
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
			return fail(NewHandlerError("failed to queue file for processing", http.StatusInternalServerError))
		}

		h.directUploads.finishFinalize(request.Key, job.ID)
		return map[string]any{"jobId": job.ID}, nil
	})
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
		if session.Multipart && !session.Completed && session.UploadID != "" {
			if err := h.s3Repository.AbortMultipartUpload(ctx, session.Key, session.UploadID); err != nil {
				log.Printf("failed to abort expired multipart upload %v: %v", session.Key, err)
			}
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
	if (strings.Contains(session.User.Username, ".") || strings.Contains(session.User.Username, "/")) {
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

func (h *uploadHandler) validateStart(request presignUploadRequest, maxSize int64) (string, string, *HandlerError) {
	fileName, handlerErr := sanitizeUploadFileName(request.FileName)
	if handlerErr != nil {
		return "", "", handlerErr
	}
	if handlerErr := h.validateUploadSize(request.Size); handlerErr != nil {
		return "", "", handlerErr
	}
	contentType := strings.TrimSpace(request.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if maxSize > 0 && request.Size > maxSize {
		return "", "", NewHandlerError(
			fmt.Sprintf("files larger than %d bytes must use the multipart upload endpoints", maxSize),
			http.StatusBadRequest,
		)
	}
	return fileName, contentType, nil
}

func (h *uploadHandler) newUploadKey(owner, fileName string) (string, *HandlerError) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		log.Printf("failed to generate direct upload key: %v", err)
		return "", NewHandlerError("failed to prepare upload", http.StatusInternalServerError)
	}
	return path.Join(directUploadPrefix, owner, base64.RawURLEncoding.EncodeToString(id), fileName), nil
}

func (h *uploadHandler) multipartSession(key, owner, uploadID string) (directUploadSession, *HandlerError) {
	session, err := h.directUploads.lookup(key, owner)
	if err != nil {
		return directUploadSession{}, NewHandlerError("unknown upload", http.StatusNotFound)
	}
	if !session.Multipart {
		return directUploadSession{}, NewHandlerError("upload is not a multipart upload", http.StatusBadRequest)
	}
	if session.UploadID != uploadID {
		return directUploadSession{}, NewHandlerError("upload id does not match the upload", http.StatusBadRequest)
	}
	return session, nil
}

func validatePartNumbers(numbers []int32, limit int) ([]int32, *HandlerError) {
	if len(numbers) == 0 {
		return nil, NewHandlerError("partNumbers must not be empty", http.StatusBadRequest)
	}
	if len(numbers) > limit {
		return nil, NewHandlerError(fmt.Sprintf("request at most %d part URLs at a time", limit), http.StatusBadRequest)
	}

	seen := make(map[int32]struct{}, len(numbers))
	for _, number := range numbers {
		if number < 1 || number > maxPartNumber {
			return nil, NewHandlerError(fmt.Sprintf("part numbers must be between 1 and %d", maxPartNumber), http.StatusBadRequest)
		}
		if _, ok := seen[number]; ok {
			return nil, NewHandlerError(fmt.Sprintf("duplicate part number %d", number), http.StatusBadRequest)
		}
		seen[number] = struct{}{}
	}
	sort.Slice(numbers, func(i, j int) bool { return numbers[i] < numbers[j] })
	return numbers, nil
}

// sanitizeUploadFileName keeps the key confined to the server prefix and the type
// confined to what the processing pipeline understands.
func sanitizeUploadFileName(raw string) (string, *HandlerError) {
	fileName := strings.TrimSpace(raw)
	switch {
	case fileName == "":
		return "", NewHandlerError("fileName is required", http.StatusBadRequest)
	case len(fileName) > 255:
		return "", NewHandlerError("fileName is too long", http.StatusBadRequest)
	case fileName == "." || fileName == ".." || strings.ContainsAny(fileName, `/\`):
		return "", NewHandlerError("fileName must not contain path separators", http.StatusBadRequest)
	case strings.ContainsFunc(fileName, func(character rune) bool { return character < 0x20 || character == 0x7f }):
		return "", NewHandlerError("fileName contains invalid characters", http.StatusBadRequest)
	case !strings.EqualFold(path.Ext(fileName), ".mcap"):
		return "", NewHandlerError("only .mcap files can be uploaded", http.StatusBadRequest)
	}
	return fileName, nil
}

// recoverDirectUploadSession rebuilds a session after a restart. It only accepts keys
// in the caller's own namespace.
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

// normalizeETag quotes the value S3 expects, including from stores that omit the quotes.
func normalizeETag(raw string) (string, error) {
	eTag := strings.TrimSpace(raw)
	if eTag == "" {
		return "", errors.New("each part must include an eTag")
	}
	if len(eTag) >= 2 && strings.HasPrefix(eTag, `"`) && strings.HasSuffix(eTag, `"`) {
		eTag = eTag[1 : len(eTag)-1]
	}
	if eTag == "" || strings.Contains(eTag, `"`) {
		return "", errors.New("eTag is invalid")
	}
	return `"` + eTag + `"`, nil
}
