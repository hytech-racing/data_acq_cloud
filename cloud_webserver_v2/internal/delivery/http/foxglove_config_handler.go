package http

import (
	"fmt"
	"log"
	"net/http"
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

/* TODO: for FoxgloveConfig handler:
   - [x] Add logic for parsing decoded FoxgloveConfig files
   - [x] Be able to send those messages out to subscribers
   - [x] Be able to write MATLAB files from the FoxgloveConfig inputs.
   - [x] Store/organize those FoxgloveConfig and Matlab files in AWS S3 (waiting on drivebrain to write FoxgloveConfig files with dates/other info in metadata)
   - [x] After debugging, make UploadFoxgloveConfig route quickly give response and perform task after responding
   - [ ] The interpolation logic is a little flawed. More docs on that is in the bookstack. We need to fix it but it is low-priority for now.
   - [ ] Once interpolation logic is fixed, write an interpolated FoxgloveConfig file with the data.
*/

// foxgloveConfigHandler handles all requests related to FoxgloveConfig data (uploads, deltions, edits, reading).
type foxgloveConfigHandler struct {
	s3Repository  *s3.S3Repository
	dbClient      *database.DatabaseClient
	fileProcessor *background.FileProcessor
}

func NewfoxgloveConfigHandler(
	r *chi.Mux,
	s3Repository *s3.S3Repository,
	dbClient *database.DatabaseClient,
	fileProcessor *background.FileProcessor,
	fileUploadMiddleware *hytech_middleware.FileUploadMiddleware,
	mpsClient *mps.MatlabClient,
) {
	handler := &foxgloveConfigHandler{
		s3Repository:  s3Repository,
		dbClient:      dbClient,
		fileProcessor: fileProcessor,
	}

	r.Route("/foxgloveconfigs", func(r chi.Router) {
		// The FileUploadMiddleware is attached to all routes involved with uploading files
		// It limits the amount of uploads we accept to a pre-set limit
		r.With(fileUploadMiddleware.FileUploadSizeLimitMiddleware).Post("/upload", HandlerFunc(handler.UploadFoxgloveConfig).ServeHTTP)
		r.Get("/", handler.GetFoxgloveConfigs)
		r.Get("/{id}", HandlerFunc(handler.GetFoxgloveConfigFromID).ServeHTTP)
		r.Delete("/{id}", HandlerFunc(handler.DeleteFoxgloveConfigFromID).ServeHTTP)
	})
}

// GetFoxgloveConfigsFromFilters takes in filters through Query parameters and will respond with a
// map with a message and data field where data contains the filtered FoxgloveConfigs
func (h *foxgloveConfigHandler) GetFoxgloveConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var resModels []models.FoxgloveConfigModel
	var err error

	resModels, err = h.dbClient.FoxgloveConfigUseCase().GetAllFoxgloveConfigs(ctx)

	if err != nil {
		log.Fatal(err)
	}

	res := make([]models.FoxgloveConfigModelResponse, len(resModels))
	for idx, model := range resModels {
		res[idx] = models.FoxgloveConfigSerialize(ctx, h.s3Repository, model)
	}

	data := make(map[string]interface{})
	data["data"] = res
	data["message"] = make(map[string]interface{})
	render.JSON(w, r, data)
}

// GetFoxgloveConfigFromID takes in an ID from a URL param and responds with an FoxgloveConfig with that ID.
func (h *foxgloveConfigHandler) GetFoxgloveConfigFromID(w http.ResponseWriter, r *http.Request) *HandlerError {
	ctx := r.Context()

	foxgloveconfigId := chi.URLParam(r, "id")
	if foxgloveconfigId == "" {
		return NewHandlerError("invalid request, must pass in foxgloveconfig id", http.StatusBadRequest)
	}

	objectId, err := primitive.ObjectIDFromHex(foxgloveconfigId)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("could not decode foxgloveconfig id %v, %v", foxgloveconfigId, err), http.StatusInternalServerError)
	}

	foxgloveconfig, err := h.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, objectId)
	if err != nil {
		if err.Error() == "mongo: no documents in result" {
			return NewHandlerError(fmt.Sprintf("no run with id %v found", foxgloveconfigId), http.StatusNotFound)
		}
		return NewHandlerError(err.Error(), http.StatusInternalServerError)
	}
	responseFoxgloveConfig := models.VehicleRunSerialize(ctx, h.s3Repository, *foxgloveconfig)
	data := make([]models.VehicleRunModelResponse, 1)
	data[0] = responseFoxgloveConfig

	response := make(map[string]interface{})
	response["message"] = ""
	response["data"] = data

	render.JSON(w, r, response)

	return nil
}

// UploadFoxgloveConfig allows for a single FoxgloveConfig file upload and enqueues the job in the FileProcessor
func (h *foxgloveConfigHandler) UploadFoxgloveConfig(w http.ResponseWriter, r *http.Request) *HandlerError {
	ctx := r.Context()
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("Failed to parse multipart form"), http.StatusBadRequest)
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return NewHandlerError(fmt.Sprintf("Could not read file from request"), http.StatusBadRequest)
	}
	defer file.Close()

	s3FileName := fmt.Sprintf("foxgloveConfigs/%s", header.Filename)
	err = h.s3Repository.WriteObjectReader(ctx, file, s3FileName)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("Failed to upload to S3: "+err.Error()), http.StatusInternalServerError)
	}

	response := make(map[string]interface{})
	response["message"] = "upload was successful"

	render.JSON(w, r, response)
	return nil
}

// DeleteFoxgloveConfigFromID takes in an ID from a URL param and deletes the FoxgloveConfig information from MongoDB and from S3.
func (h *foxgloveConfigHandler) DeleteFoxgloveConfigFromID(w http.ResponseWriter, r *http.Request) *HandlerError {
	ctx := r.Context()

	foxgloveconfigId := chi.URLParam(r, "id")
	if foxgloveconfigId == "" {
		return NewHandlerError("invalid request, must pass in foxgloveconfig id", http.StatusBadRequest)
	}

	objectId, err := primitive.ObjectIDFromHex(foxgloveconfigId)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("could not decode mcap id %v, %v", foxgloveconfigId, err), http.StatusInternalServerError)
	}

	foxgloveConfigModel, err := h.dbClient.FoxgloveConfigUseCase().GetFoxgloveConfigFromId(ctx, foxgloveconfigId)
	if err != nil {
		if err.Error() == "mongo: no documents in result" {
			return NewHandlerError(fmt.Sprintf("no foxgloveConfig with id %v found", foxgloveconfigId), http.StatusNotFound)
		}
		return NewHandlerError(err.Error(), http.StatusInternalServerError)
	}

	err = h.s3Repository.DeleteObject(ctx, foxgloveConfigModel.File.AwsBucket, foxgloveConfigModel.File.FilePath)
	if err != nil {
		return NewHandlerError(err.Error(), http.StatusInternalServerError)
	}

	err = h.dbClient.FoxgloveConfigUseCase().DeleteFoxgloveConfigFromId(ctx, objectId)
	if err != nil {
		return NewHandlerError(err.Error(), http.StatusInternalServerError)
	}

	return nil
}

// UpdateMetadataRecordFromID takes in an ID from a URL param and formdata that determines which metadata to update in our VehicleRunModels.
func (h *foxgloveConfigHandler) UpdateMetadataRecordFromID(w http.ResponseWriter, r *http.Request) *HandlerError {
	ctx := r.Context()
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		return NewHandlerError("error parsing form data", http.StatusBadRequest)
	}
	defer r.MultipartForm.RemoveAll()

	foxgloveconfigId := chi.URLParam(r, "id")
	if foxgloveconfigId == "" {
		return NewHandlerError("invalid request, must pass in foxgloveconfig id", http.StatusBadRequest)
	}

	objectId, err := primitive.ObjectIDFromHex(foxgloveconfigId)
	if err != nil {
		log.Println("getting foxgloveconfigID")
		return NewHandlerError(fmt.Sprintf("could not decode foxgloveconfig id %v, %v", foxgloveconfigId, err), http.StatusInternalServerError)
	}

	runModel, err := h.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, objectId)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("could not get vehicle run by id %v, %v", foxgloveconfigId, err), http.StatusInternalServerError)
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
func (h *foxgloveConfigHandler) ResetMetadataRecordFromID(w http.ResponseWriter, r *http.Request) *HandlerError {
	ctx := r.Context()
	foxgloveconfigId := chi.URLParam(r, "id")
	if foxgloveconfigId == "" {
		return NewHandlerError("invalid request, must pass in foxgloveconfig id", http.StatusBadRequest)
	}
	metadata := chi.URLParam(r, "metadata")
	if metadata == "" {
		return NewHandlerError("invalid request, must pass in foxgloveconfig id", http.StatusBadRequest)
	}
	objectId, err := primitive.ObjectIDFromHex(foxgloveconfigId)
	if err != nil {
		log.Println("getting foxgloveconfigID")
		return NewHandlerError(fmt.Sprintf("could not decode foxgloveconfig id %v, %v", foxgloveconfigId, err), http.StatusInternalServerError)
	}

	runModel, err := h.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, objectId)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("could not get vehicle run by id %v, %v", foxgloveconfigId, err), http.StatusInternalServerError)
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
