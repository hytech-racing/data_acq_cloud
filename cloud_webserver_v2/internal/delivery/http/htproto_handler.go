package http

import (
	"io/ioutil"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

type docHandler struct{}

// Create new handler
func NewDocHandler(r *chi.Mux) {
	handler := &docHandler{}

	r.Route("/docs", func(r chi.Router) {
		r.Get("/versions", handler.GetVersions)
		r.Get("/versions/{version_name}", HandlerFunc(handler.GetVersionFromName).ServeHTTP)
	})
}

// Get all version names 
func (*docHandler) GetVersions(w http.ResponseWriter, r *http.Request)  {

	versions := []string{}

	files, err := ioutil.ReadDir("/app/files")
	if err != nil {
        log.Println(err)
    }

    for _, file := range files {
		versions = append(versions, file.Name())
    }

	response := make(map[string]interface{})
	response["message"] = "returned all doc versions"
	response["data"] = versions

	render.JSON(w, r, response)
}

// Get HTML from inputted version name
func (*docHandler) GetVersionFromName(w http.ResponseWriter, r *http.Request) *HandlerError {

	versionName := chi.URLParam(r, "version_name")
	if versionName == "" {
		return NewHandlerError("invalid request, must pass in version name", http.StatusBadRequest)
	}

	filePath := "/app/files/" + versionName

	htmlContent, err := ioutil.ReadFile(filePath)
	if err != nil {
		return NewHandlerError("failed to read HTML file: "+err.Error(), http.StatusInternalServerError)
	}
	
	response := make(map[string]interface{})
	response["message"] = "returned specified doc"

	// Returns html as string
	response["HTML"] = string(htmlContent)
	
	render.JSON(w, r, response)
	return nil
}