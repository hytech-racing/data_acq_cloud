package http

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/hytech-racing/cloud-webserver-v2/internal/background"
	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
	hytech_middleware "github.com/hytech-racing/cloud-webserver-v2/internal/middleware"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"github.com/hytech-racing/cloud-webserver-v2/internal/mps"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

/* TODO: for MCAP handler:
   - [x] Add logic for parsing decoded MCAP files
   - [x] Be able to send those messages out to subscribers
   - [x] Be able to write MATLAB files from the MCAP inputs.
   - [x] Store/organize those MCAP and Matlab files in AWS S3 (waiting on drivebrain to write MCAP files with dates/other info in metadata)
   - [x] After debugging, make UploadMcap route quickly give response and perform task after responding
   - [ ] The interpolation logic is a little flawed. More docs on that is in the bookstack. We need to fix it but it is low-priority for now.
   - [ ] Once interpolation logic is fixed, write an interpolated MCAP file with the data.
*/

// mcapHandler handles all requests related to MCAP data (uploads, deltions, edits, reading).
type mcapHandler struct {
	s3Repository  *s3.S3Repository
	dbClient      *database.DatabaseClient
	fileProcessor *background.FileProcessor
	mpsClient     *mps.MatlabClient
}

func NewMcapHandler(
	r *chi.Mux,
	s3Repository *s3.S3Repository,
	dbClient *database.DatabaseClient,
	fileProcessor *background.FileProcessor,
	fileUploadMiddleware *hytech_middleware.FileUploadMiddleware,
	mpsClient *mps.MatlabClient,
) {
	handler := &mcapHandler{
		s3Repository:  s3Repository,
		dbClient:      dbClient,
		fileProcessor: fileProcessor,
		mpsClient:     mpsClient,
	}

	r.Route("/mcaps", func(r chi.Router) {
		// The FileUploadMiddleware is attached to all routes involved with uploading files
		// It limits the amount of uploads we accept to a pre-set limit
		r.With(fileUploadMiddleware.FileUploadSizeLimitMiddleware).Post("/upload", handler.UploadMcap)
		r.With(fileUploadMiddleware.FileUploadSizeLimitMiddleware).Post("/bulk_upload", handler.BulkUploadMcaps)
		r.Get("/", handler.GetMcapsFromFilters)
		r.Get("/{id}", HandlerFunc(handler.GetMcapFromID).ServeHTTP)
		r.Delete("/{id}", HandlerFunc(handler.DeleteMcapFromID).ServeHTTP)
		r.Get("/{id}/process", HandlerFunc(handler.ProcessMatlabJob).ServeHTTP)
		r.Post("/{id}/updateMetadataRecords", HandlerFunc(handler.UpdateMetadataRecordFromID).ServeHTTP)
		r.Delete("/{id}/resetMetaDataRecord/{metadata}", HandlerFunc(handler.ResetMetadataRecordFromID).ServeHTTP)
	})
}

// GetMcapsFromFilters takes in filters through Query parameters and will respond with a
// map with a message and data field where data contains the filtered MCAPs
func (h *mcapHandler) GetMcapsFromFilters(w http.ResponseWriter, r *http.Request) {
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

	if queryParams.Has("mps_function") {
		mps_function := queryParams.Get("mps_function")
		filters.MpsFunction = &mps_function
	}

	var resModels []models.VehicleRunModel
	var err error

	if queryParams.Has("limit") && queryParams.Has("offset") {
		limit, errLimit := strconv.ParseInt(queryParams.Get("limit"), 10, 64)
		offset, errOffset := strconv.ParseInt(queryParams.Get("offset"), 10, 64)

		if errLimit == nil && errOffset == nil {
			resModels, err = h.dbClient.VehicleRunUseCase().GetVehicleRunByFiltersPaged(ctx, &filters, limit, offset)
		} else {
			resModels, err = h.dbClient.VehicleRunUseCase().GetVehicleRunByFilters(ctx, &filters)
		}
	} else {
		resModels, err = h.dbClient.VehicleRunUseCase().GetVehicleRunByFilters(ctx, &filters)
	}

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

// GetMcapFromID takes in an ID from a URL param and responds with an MCAP with that ID.
func (h *mcapHandler) GetMcapFromID(w http.ResponseWriter, r *http.Request) *HandlerError {
	ctx := r.Context()

	mcapId := chi.URLParam(r, "id")
	if mcapId == "" {
		return NewHandlerError("invalid request, must pass in mcap id", http.StatusBadRequest)
	}

	objectId, err := primitive.ObjectIDFromHex(mcapId)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("could not decode mcap id %v, %v", mcapId, err), http.StatusInternalServerError)
	}

	mcap, err := h.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, objectId)
	if err != nil {
		if err.Error() == "mongo: no documents in result" {
			return NewHandlerError(fmt.Sprintf("no run with id %v found", mcapId), http.StatusNotFound)
		}
		return NewHandlerError(err.Error(), http.StatusInternalServerError)
	}
	responseMcap := models.VehicleRunSerialize(ctx, h.s3Repository, *mcap)
	data := make([]models.VehicleRunModelResponse, 1)
	data[0] = responseMcap

	response := make(map[string]interface{})
	response["message"] = ""
	response["data"] = data

	render.JSON(w, r, response)

	return nil
}

// UploadMcap allows for a single MCAP file upload and enqueues the job in the FileProcessor
func (h *mcapHandler) UploadMcap(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	file := r.MultipartForm.File["file"]
	jobIds := make([]string, 1, len(file))
	fileHeader := file[0]
	job, err := h.fileProcessor.EnqueueFile(fileHeader, &background.PostProcessMCAPUploadJob{})
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

// BulkUploadMcap allows for a many MCAP file uploads and enqueues the jobs in the FileProcessor
func (h *mcapHandler) BulkUploadMcaps(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	files := r.MultipartForm.File["files"]
	jobIds := make([]string, 0, len(files))
	for _, fileHeader := range files {
		job, err := h.fileProcessor.EnqueueFile(fileHeader, &background.PostProcessMCAPUploadJob{})
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

// DeleteMcapFromID takes in an ID from a URL param and deletes the MCAP information from MongoDB and from S3.
func (h *mcapHandler) DeleteMcapFromID(w http.ResponseWriter, r *http.Request) *HandlerError {
	ctx := r.Context()

	mcapId := chi.URLParam(r, "id")
	if mcapId == "" {
		return NewHandlerError("invalid request, must pass in mcap id", http.StatusBadRequest)
	}

	objectId, err := primitive.ObjectIDFromHex(mcapId)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("could not decode mcap id %v, %v", mcapId, err), http.StatusInternalServerError)
	}

	vehicleModel, err := h.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, objectId)
	if err != nil {
		if err.Error() == "mongo: no documents in result" {
			return NewHandlerError(fmt.Sprintf("no run with id %v found", mcapId), http.StatusNotFound)
		}
		return NewHandlerError(err.Error(), http.StatusInternalServerError)
	}

	for _, mcapFileModel := range vehicleModel.McapFiles {
		err = h.s3Repository.DeleteObject(ctx, mcapFileModel.AwsBucket, mcapFileModel.FilePath)
		if err != nil {
			return NewHandlerError(err.Error(), http.StatusInternalServerError)
		}
	}

	for _, matFileModel := range vehicleModel.MatFiles {
		err = h.s3Repository.DeleteObject(ctx, matFileModel.AwsBucket, matFileModel.FilePath)
		if err != nil {
			return NewHandlerError(err.Error(), http.StatusInternalServerError)
		}
	}

	for _, contentFileModels := range vehicleModel.ContentFiles {
		for _, content := range contentFileModels {
			err = h.s3Repository.DeleteObject(ctx, content.AwsBucket, content.FilePath)
			if err != nil {
				return NewHandlerError(err.Error(), http.StatusInternalServerError)
			}
		}
	}

	err = h.dbClient.VehicleRunUseCase().DeleteVehicleRunById(ctx, objectId)
	if err != nil {
		return NewHandlerError(err.Error(), http.StatusInternalServerError)
	}

	return nil
}

func (h *mcapHandler) ProcessMatlabJob(w http.ResponseWriter, r *http.Request) *HandlerError {
	ctx := r.Context()

	scriptsParam := r.URL.Query().Get("scripts")
	if scriptsParam == "" {
		return NewHandlerError("invalid request, must pass in query param scripts with a value of comma seperated script names", http.StatusBadRequest)
	}
	scripts := strings.Split(scriptsParam, ",")

	versionParam := r.URL.Query().Get("version")
	if versionParam == "" {
		return NewHandlerError("invalid request, must pass in archive version in query params", http.StatusBadRequest)
	}

	mcapId := chi.URLParam(r, "id")
	if mcapId == "" {
		return NewHandlerError("invalid request, must pass in mcap id", http.StatusBadRequest)
	}

	objectId, err := primitive.ObjectIDFromHex(mcapId)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("could not decode mcap id %v, %v", mcapId, err), http.StatusInternalServerError)
	}

	mcap, err := h.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, objectId)
	if err != nil {
		if err.Error() == "mongo: no documents in result" {
			return NewHandlerError(fmt.Sprintf("no run with id %v found", mcapId), http.StatusNotFound)
		}
		return NewHandlerError(err.Error(), http.StatusInternalServerError)
	}
	responseMcap := models.VehicleRunSerialize(ctx, h.s3Repository, *mcap)

	matFiles := responseMcap.MatFiles

	if len(matFiles) == 0 {
		return NewHandlerError("no h5 files found", http.StatusFailedDependency)
	}

	for _, script := range scripts {
		h.mpsClient.SubmitMatlabJob(ctx, h.s3Repository, mcapId, versionParam, script)
	}

	render.JSON(w, r, "jobs submitted")

	return nil
}

// UpdateMetadataRecordFromID takes in an ID from a URL param and formdata that determines which metadata to update in our VehicleRunModels.
func (h *mcapHandler) UpdateMetadataRecordFromID(w http.ResponseWriter, r *http.Request) *HandlerError {
	ctx := r.Context()
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		return NewHandlerError("error parsing form data", http.StatusBadRequest)
	}
	defer r.MultipartForm.RemoveAll()

	mcapId := chi.URLParam(r, "id")
	if mcapId == "" {
		return NewHandlerError("invalid request, must pass in mcap id", http.StatusBadRequest)
	}

	objectId, err := primitive.ObjectIDFromHex(mcapId)
	if err != nil {
		log.Println("getting mcapID")
		return NewHandlerError(fmt.Sprintf("could not decode mcap id %v, %v", mcapId, err), http.StatusInternalServerError)
	}

	runModel, err := h.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, objectId)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("could not get vehicle run by id %v, %v", mcapId, err), http.StatusInternalServerError)
	}

	for key, values := range r.Form {
		if strings.HasPrefix(key, "mps.") {
			// TODO: Figure out if updating MPS with an HTTP request is needed

			// mpsMetadata := make(map[string]interface{})
			// mpsMetadata[strings.TrimPrefix(key, "mps.")] = values[0]

			// if runModel.MpsRecord == nil {
			// 	runModel.MpsRecord = make(map[string]models.MpsScripts)
			// }

			// for function, record := range mpsMetadata {
			// 	runModel.MpsRecord[function] = record
			// }
		} else {
			switch key {
			case "date":
				layout := time.RFC3339
				parsedDate, err := time.Parse(layout, values[0])
				if err != nil {
					return NewHandlerError(fmt.Sprintf("invalid date format: %v", err), http.StatusBadRequest)
				}
				runModel.Date = parsedDate
			case "location":
				runModel.Location = &values[0]
			case "notes":
				runModel.Notes = &values[0]
			case "event_type":
				runModel.EventType = &values[0]
			case "car_model":
				runModel.CarModel = values[0]
			}
		}
	}

	err = h.dbClient.VehicleRunUseCase().UpdateVehicleRun(ctx, objectId, runModel)
	if err != nil {
		return NewHandlerError(err.Error(), http.StatusInternalServerError)
	}

	return nil
}

// ResetMetadataRecordFromID takes in an ID and metadata from a URL param that determines which metadata to reset in our VehicleRunModels.
func (h *mcapHandler) ResetMetadataRecordFromID(w http.ResponseWriter, r *http.Request) *HandlerError {
	ctx := r.Context()
	mcapId := chi.URLParam(r, "id")
	if mcapId == "" {
		return NewHandlerError("invalid request, must pass in mcap id", http.StatusBadRequest)
	}
	metadata := chi.URLParam(r, "metadata")
	if metadata == "" {
		return NewHandlerError("invalid request, must pass in mcap id", http.StatusBadRequest)
	}
	objectId, err := primitive.ObjectIDFromHex(mcapId)
	if err != nil {
		log.Println("getting mcapID")
		return NewHandlerError(fmt.Sprintf("could not decode mcap id %v, %v", mcapId, err), http.StatusInternalServerError)
	}

	runModel, err := h.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, objectId)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("could not get vehicle run by id %v, %v", mcapId, err), http.StatusInternalServerError)
	}

	switch metadata {
	case "date":
		runModel.Date = time.Now()
	case "note":
		runModel.Notes = nil
	case "location":
		runModel.Location = nil
	case "event_type":
		runModel.EventType = nil
	// TODO: Figure out if updating MPS with an HTTP request is needed
	// case "mps_record":
	// 	runModel.MpsRecord = make(map[string]interface{})
	case "car_model":
		runModel.CarModel = ""
	default:
		return NewHandlerError("invalid metadata key", http.StatusBadRequest)
	}

	err = h.dbClient.VehicleRunUseCase().UpdateVehicleRun(ctx, objectId, runModel)
	if err != nil {
		return NewHandlerError(err.Error(), http.StatusInternalServerError)
	}
	return nil
}
