package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
		r.Get("/versions/{version_name}/{repo}", HandlerFunc(handler.GetVersionFromName).ServeHTTP)
		r.Get("/request/{repo}/{release}", HandlerFunc(handler.GetDocumentationFromRelease).ServeHTTP)
	})
}

// Get documentation HTML from a specific release of a repository
func (d *documentationHandler) GetDocumentationFromRelease(w http.ResponseWriter, r *http.Request) *HandlerError {
	repo := chi.URLParam(r, "repo")
	release := chi.URLParam(r, "release")

	if repo == "" || release == "" {
		return NewHandlerError("invalid request, must pass in both repo and release", http.StatusBadRequest)
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/hytech-racing/%s/releases/tags/%s", repo, release)

	resp, err := http.Get(apiURL)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("Error fetching release info from GitHub: %s", err.Error()), http.StatusInternalServerError)
	}
	defer resp.Body.Close()

	var releaseInfo struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	err = json.NewDecoder(resp.Body).Decode(&releaseInfo)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("Error parsing release info from GitHub: %s", err.Error()), http.StatusInternalServerError)
	}

	var htmlFileURL string
	for _, asset := range releaseInfo.Assets {
		if strings.HasSuffix(asset.Name, ".html") {
			htmlFileURL = asset.URL
			break
		}
	}

	if htmlFileURL == "" {
		return NewHandlerError("No HTML file found in the release", http.StatusNotFound)
	}

	resp, err = http.Get(htmlFileURL)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("Error fetching HTML file from GitHub: %s", err.Error()), http.StatusInternalServerError)
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "text/html")

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		return NewHandlerError(fmt.Sprintf("Error writing HTML file to response: %s", err.Error()), http.StatusInternalServerError)
	}

	return nil
}

// Get all version names
func (d *documentationHandler) GetVersions(w http.ResponseWriter, r *http.Request) *HandlerError {
	versionsCAN := []string{}
	versionsProto := []string{}

	// Returns all entries in external directory
	filePathCAN := filepath.Join("/app/files/", "HT_CAN")

	canFiles, err := os.ReadDir(filePathCAN)
	if err != nil {
		return NewHandlerError("failed to read external directory [~/htmls]: "+err.Error(), http.StatusInternalServerError)
	}

	for _, canFile := range canFiles {
		versionsCAN = append(versionsCAN, canFile.Name())
	}

	filePathProto := filepath.Join("/app/files/", "HT_proto")

	protoFiles, err := os.ReadDir(filePathProto)
	if err != nil {
		return NewHandlerError("failed to read external directory [~/htmls]: "+err.Error(), http.StatusInternalServerError)
	}

	for _, protoFile := range protoFiles {
		versionsProto = append(versionsProto, protoFile.Name())
	}

	response := make(map[string]interface{})
	response["message"] = "returned all doc versions"
	response["HT_CAN"] = versionsCAN
	response["HT_Proto"] = versionsProto

	render.JSON(w, r, response)
	return nil
}

// Get HTML from inputted version name
func (d *documentationHandler) GetVersionFromName(w http.ResponseWriter, r *http.Request) *HandlerError {
	ctx := r.Context()

	versionName := chi.URLParam(r, "version_name")
	repo := chi.URLParam(r, "repo")

	if versionName == "" {
		return NewHandlerError("invalid request, must pass in version name", http.StatusBadRequest)
	}

	filePath := filepath.Join("/app/files/", repo, versionName)

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

		// Create Filepath to put the file into the external mount to prevent future error *(Pulling from s3 if not in external mount)
		out, err := os.Create(filePath)
		if err != nil {
			return NewHandlerError("Error in creating new file", http.StatusBadRequest)
		}
		defer out.Close()

		// Write contents onto the created file to prevent future error *(Pulling from s3 if not in external mount)
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			return NewHandlerError("Error in writing html content to file", http.StatusBadRequest)
		}

		return nil
	}

	response := make(map[string]interface{})

	// Returns html as string
	response["HTML"] = string(htmlContent)

	render.JSON(w, r, response)
	return nil
}
