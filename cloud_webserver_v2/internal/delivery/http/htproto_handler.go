package http

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
)

type documentationHandler struct {
	s3Repository *s3.S3Repository
}

// Create new handler
func NewDocumentationHandler(r *chi.Mux, s3Repository *s3.S3Repository) {
	handler := &documentationHandler{
		s3Repository: s3Repository,
	}

	r.Route("/docs", func(r chi.Router) {
		r.Get("/versions", HandlerFunc(handler.GetVersions).ServeHTTP)
		r.Get("/versions/{version_name}", HandlerFunc(handler.GetVersionFromName).ServeHTTP)
	})
}

// Get all version names
func (*documentationHandler) GetVersions(w http.ResponseWriter, r *http.Request) *HandlerError {
	versions := []string{}

	// Returns all entries in external directory
	files, err := os.ReadDir("/app/files")
	if err != nil {
		return NewHandlerError("failed to read external directory [~/htmls]: "+err.Error(), http.StatusInternalServerError)
	}

	for _, file := range files {
		versions = append(versions, file.Name())
	}

	response := make(map[string]interface{})
	response["message"] = "returned all doc versions"
	response["data"] = versions

	render.JSON(w, r, response)
	return nil
}

// Get HTML from inputted version name
func (d *documentationHandler) GetVersionFromName(w http.ResponseWriter, r *http.Request) *HandlerError {
	ctx := r.Context()

	versionName := chi.URLParam(r, "version_name")
	if versionName == "" {
		return NewHandlerError("invalid request, must pass in version name", http.StatusBadRequest)
	}

	filePath := filepath.Join("/app/files/", versionName)

	// Read specified file content
	htmlContent, err := os.ReadFile(filePath)
	if err != nil {
		//  Pull from s3 if not in external mount
		signedURL := d.s3Repository.GetSignedUrl(ctx, d.s3Repository.Bucket(), versionName)

		resp, err := http.Get(signedURL)
		if err != nil {
			return NewHandlerError("No file found in s3 OR external mount", http.StatusBadRequest)
		}
		defer resp.Body.Close()

		htmlContent, err := io.ReadAll(resp.Body)
		if err != nil {
			return NewHandlerError("Error reading s3 file", http.StatusBadRequest)
		}

		response := make(map[string]interface{})

		// Returns html as string
		response["HTML"] = string(htmlContent)

		render.JSON(w, r, response)
		return nil
	}

	response := make(map[string]interface{})

	// Returns html as string
	response["HTML"] = string(htmlContent)

	render.JSON(w, r, response)
	return nil
}
