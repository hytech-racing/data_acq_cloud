package http

import (
	"context"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/hytech-racing/cloud-webserver-v2/internal/background"
	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
	"github.com/hytech-racing/cloud-webserver-v2/internal/messaging"
	hytech_middleware "github.com/hytech-racing/cloud-webserver-v2/internal/middleware"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
)

/* TODO: for MCAP handler:
   - [x] Add logic for parsing decoded MCAP files
   - [x] Be able to send those messages out to subscribers
   - [x] Be able to write MATLAB files from the MCAP inputs.
   - [ ] Store/organize those MCAP and Matlab files in AWS S3 (waiting on drivebrain to write MCAP files with dates/other info in metadata)
   - [ ] After debugging, make UploadMcap route quickly give response and perform task after responding
   - [ ] The interpolation logic is a little flawed. More docs on that is in the bookstack. We need to fix it but it is low-priority for now.
   - [ ] Once interpolation logic is fixed, write an interpolated MCAP file with the data.
*/

type mcapHandler struct {
	s3Repository  *s3.S3Repository
	dbClient      *database.DatabaseClient
	fileProcessor *background.FileProcessor
}

func NewMcapHandler(
	r *chi.Mux,
	s3Repository *s3.S3Repository,
	dbClient *database.DatabaseClient,
	fileProcessor *background.FileProcessor,
	fileUploadMiddleware *hytech_middleware.FileUploadMiddleware,
) {
	handler := &mcapHandler{
		s3Repository:  s3Repository,
		dbClient:      dbClient,
		fileProcessor: fileProcessor,
	}

	r.Route("/mcaps", func(r chi.Router) {
		r.With(fileUploadMiddleware.FileUploadSizeLimitMiddleware).Post("/upload", handler.UploadMcap)
		r.With(fileUploadMiddleware.FileUploadSizeLimitMiddleware).Post("/bulk_upload", handler.BulkUploadMcaps)
	})
}

/*
This route takes an MCAP file and performs a series of actions on it.
It currently:
  - plots the vectornav lat/lon onto a cartesian plane
  - creates an interpolated MATLAB file
  - creates a raw (no calculations performed onto it) MATLAB file
*/
func (h *mcapHandler) UploadMcap(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	file := r.MultipartForm.File["file"]
	jobIds := make([]string, 1, len(file))
	fileHeader := file[0]
	job, err := h.fileProcessor.QueueFile(fileHeader)
	if err != nil {
		log.Printf("Failed to queue file %s: %v", fileHeader.Filename, err)
		return
	}
	jobIds[0] = job.ID

	response := make(map[string]interface{})
	response["message"] = "created file processing job"
	response["data"] = jobIds

	render.JSON(w, r, response)
}

func (h *mcapHandler) routeMessagesToSubscribers(ctx context.Context, publisher *messaging.Publisher, decodedMessage *utils.DecodedMessage, allNames *[]string) {
	// List of all the workers we want to send the messages to
	var subscriberNames []string
	switch topic := decodedMessage.Topic; topic {
	case messaging.EOF:
		subscriberNames = append(subscriberNames, *allNames...)
	case "vn_lat_lon":
		subscriberNames = append(subscriberNames, "vn_plot", "matlab_writer")
	default:
		subscriberNames = append(subscriberNames, "print")
	}

	publisher.Publish(ctx, decodedMessage, subscriberNames)
}

func (h *mcapHandler) BulkUploadMcaps(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	files := r.MultipartForm.File["files"]
	jobIds := make([]string, 0, len(files))
	for _, fileHeader := range files {
		job, err := h.fileProcessor.QueueFile(fileHeader)
		if err != nil {
			log.Printf("Failed to queue file %s: %v", fileHeader.Filename, err)
			continue
		}
		jobIds = append(jobIds, job.ID)
	}

	response := make(map[string]interface{})
	response["message"] = "created file processing jobs"
	response["data"] = jobIds

	render.JSON(w, r, response)
}
