package http

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

type documentationHandler struct{}

// Create new handler
func NewDocumentationHandler(r *chi.Mux) {
	handler := &documentationHandler{}

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
func (*documentationHandler) GetVersionFromName(w http.ResponseWriter, r *http.Request) *HandlerError {
	versionName := chi.URLParam(r, "version_name")
	if versionName == "" {
		return NewHandlerError("invalid request, must pass in version name", http.StatusBadRequest)
	}

	filePath := filepath.Join("/app/files/", versionName)

	// Read specified file content
	htmlContent, err := os.ReadFile(filePath)
	if err != nil {
		return NewHandlerError("failed to read HTML file: "+err.Error(), http.StatusInternalServerError)
	}

	response := make(map[string]interface{})

	// Returns html as string
	response["message"] = "returned specified doc"
	response["HTML"] = string(htmlContent)

	render.JSON(w, r, response)
	return nil
}
