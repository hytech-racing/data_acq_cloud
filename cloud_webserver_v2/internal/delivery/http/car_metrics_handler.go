package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
)

type carMetricsHandler struct {
	s3Repository *s3.S3Repository
	dbClient     *database.DatabaseClient
}

func NewCarMetricsHandler(
	r *chi.Mux,
	s3Repository *s3.S3Repository,
	dbClient *database.DatabaseClient,
) {
	handler := &carMetricsHandler{
		s3Repository,
		dbClient,
	}

	r.Route("/car_metrics", func(r chi.Router) {
		r.Get("/", HandlerFunc(handler.GetAllCarMetrics).ServeHTTP)
	})
}

func (h *carMetricsHandler) GetAllCarMetrics(w http.ResponseWriter, r *http.Request) *HandlerError {
	ctx := r.Context()
	resModels, err := h.dbClient.CarMetricsUseCase().GetAllCarMetrics(ctx)
	if err != nil {
		return NewHandlerError(err.Error(), http.StatusInternalServerError)
	}

	data := make(map[string]interface{})
	data["data"] = resModels
	data["message"] = "received all car models"
	render.JSON(w, r, data)
	return nil
}
