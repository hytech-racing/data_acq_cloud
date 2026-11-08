package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/hytech-racing/cloud-webserver-v2/internal/background"
	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
)

// This handles all requests dealing with managing the uploads in the server
type uploadHandler struct {
	dbClient      *database.DatabaseClient
	fileProcessor *background.FileProcessor
	s3Repository  *s3.S3Repository
	authHandler   *AuthHandler
	directUploads *directUploadRegistry
}

func NewUploadHandler(
	r *chi.Mux,
	dbClient *database.DatabaseClient,
	fileProcessor *background.FileProcessor,
	s3Repository *s3.S3Repository,
	authHandler *AuthHandler,
) {
	handler := &uploadHandler{
		dbClient:      dbClient,
		fileProcessor: fileProcessor,
		s3Repository:  s3Repository,
		authHandler:   authHandler,
		directUploads: newDirectUploadRegistry(),
	}

	r.Route("/uploads", func(r chi.Router) {
		r.Get("/limits", handler.GetUploadLimits)

		// Direct-to-S3 uploads. The client asks the server to sign requests, then sends
		// the file bytes straight to the bucket so the server never proxies them.
		r.Post("/presign", HandlerFunc(handler.PresignUpload).ServeHTTP)
		r.Post("/initiate", HandlerFunc(handler.InitiateMultipartUpload).ServeHTTP)
		r.Post("/parts", HandlerFunc(handler.PresignUploadParts).ServeHTTP)
		r.Post("/complete", HandlerFunc(handler.CompleteMultipartUpload).ServeHTTP)
		r.Post("/abort", HandlerFunc(handler.AbortMultipartUpload).ServeHTTP)
		r.Post("/finalize", HandlerFunc(handler.FinalizeUpload).ServeHTTP)
	})
}

func (handler *uploadHandler) GetUploadLimits(w http.ResponseWriter, r *http.Request) {
	currentFileSize := handler.fileProcessor.TotalSize.Load()
	maxFileSize := handler.fileProcessor.MaxTotalSize()

	data := make(map[string]interface{})
	data["current_file_size"] = currentFileSize
	data["max_file_size"] = maxFileSize
	data["available_file_size"] = maxFileSize - currentFileSize

	response := make(map[string]interface{})
	response["message"] = nil
	response["data"] = data
	render.JSON(w, r, response)
}
