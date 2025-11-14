package http

import (
	"context"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
)

// This handles all requests dealing with managing the uploads in the server
type filesHandler struct {
	dbClient     *database.DatabaseClient
	s3Repostiory *s3.S3Repository
}

type fileStatusResponse struct {
	Stored bool `json:"stored"`
}

func NewFilesHandler(
	r *chi.Mux,
	dbClient *database.DatabaseClient,
	s3Repostiory *s3.S3Repository) {
	handler := &filesHandler{
		dbClient:     dbClient,
		s3Repostiory: s3Repostiory,
	}

	r.Route("/files", func(r chi.Router) {
		r.Get("/status", handler.CheckFileStatusEndPoint)
	})
}

// GET endpoint to check if run with fileHash exists in MongoDB; params -> (file_hash, string)
func (handler *filesHandler) CheckFileStatusEndPoint(w http.ResponseWriter, r *http.Request) {
	fileHash := r.URL.Query().Get("file_hash")
	if fileHash == "" {
		http.Error(w, "file_hash must not be empty", http.StatusBadRequest)
		return
	}
	hashExists, err := handler.CheckFileStatus(r.Context(), fileHash)
	if err != nil {
		log.Printf("error checking file status for %q: %v", fileHash, err)
		http.Error(w, "internal error while checking file status", http.StatusInternalServerError)
		return
	}
	data := make(map[string]interface{})
	data["stored"] = hashExists
	render.JSON(w, r, fileStatusResponse{Stored: hashExists})
}

// Helper function to check if run with fileHash exists in MongoDB
func (handler *filesHandler) CheckFileStatus(ctx context.Context, fileHash string) (run_exists bool, err error) {
	vehicle_runs, err := handler.dbClient.VehicleRunUseCase().FindVehicleRunByMCAPFileHash(context.TODO(), fileHash)

	if err != nil {
		return false, err
	}
	return len(vehicle_runs) > 0, nil
}
