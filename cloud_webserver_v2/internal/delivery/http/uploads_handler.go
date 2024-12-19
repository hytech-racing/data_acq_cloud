package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/hytech-racing/cloud-webserver-v2/internal/background"
	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
)

// This handles all requests dealing with managing the uploads in the server
type uploadHandler struct {
	dbClient      *database.DatabaseClient
	fileProcessor *background.FileProcessor
}

func NewUploadHandler(
	r *chi.Mux,
	dbClient *database.DatabaseClient,
	fileProcessor *background.FileProcessor,
) {
	handler := &uploadHandler{
		dbClient:      dbClient,
		fileProcessor: fileProcessor,
	}

	r.Route("/uploads", func(r chi.Router) {
		r.Get("/limits", handler.GetUploadLimits)
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
