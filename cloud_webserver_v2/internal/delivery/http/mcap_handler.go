package http

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/hytech-racing/cloud-webserver-v2/internal/background"
	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
	"github.com/hytech-racing/cloud-webserver-v2/internal/messaging"
	hytech_middleware "github.com/hytech-racing/cloud-webserver-v2/internal/middleware"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
		r.Get("/", handler.GetMcaps)
	})
}

func (h *mcapHandler) GetMcaps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queryParams := r.URL.Query()

	filters := models.VehicleRunModelFilters{}

	if queryParams.Has("id") {
		id, err := primitive.ObjectIDFromHex(queryParams.Get("id"))
		if err == nil {
			filters.ID = &id
		}
	}

	utcFormat := "2006-01-02T15:04:05Z07:00"
	if queryParams.Has("before_date") {
		beforeDate := queryParams.Get("before_date")
		parsedBeforeDate, err := time.Parse(utcFormat, beforeDate)
		if err == nil {
			filters.BeforeDate = &parsedBeforeDate
		}
	}

	if queryParams.Has("after_date") {
		afterDate := queryParams.Get("after_date")
		parsedAfterDate, err := time.Parse(utcFormat, afterDate)
		if err == nil {
			filters.AfterDate = &parsedAfterDate
		}
	}

	if queryParams.Has("location") {
		location := queryParams.Get("location")
		filters.Location = &location
	}

	if queryParams.Has("event_type") {
		eventType := queryParams.Get("event_type")
		filters.EventType = &eventType
	}

	if queryParams.Has("car_model") {
		carModel := queryParams.Get("car_model")
		filters.CarModel = &carModel
	}

	if queryParams.Has("search_text") {
		search_text := queryParams.Get("search_text")
		filters.SearchText = &search_text
	}

	resModels, err := h.dbClient.VehicleRunUseCase().GetVehicleRunByFilters(ctx, &filters)
	if err != nil {
		log.Fatal(err)
	}

	res := make([]models.VehicleRunModelResponse, len(resModels))
	for idx, model := range resModels {
		res[idx] = models.VehicleRunSerialize(ctx, h.s3Repository, model)
	}

	data := make(map[string]interface{})
	data["data"] = res
	data["message"] = make(map[string]interface{})
	render.JSON(w, r, data)
}

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
