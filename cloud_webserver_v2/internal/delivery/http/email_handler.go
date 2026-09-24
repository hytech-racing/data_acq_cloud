package http

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
) 

const EMAILS_FILE = "emails.json"

type emailHandler struct {
	fileLock sync.RWMutex
}

// NewEmailHandler registers the routes used to manage the email allowlist. Login is not
// required on these routes, so the allowlist can be seeded by an administrator:
//
//	POST /emails -> register a new email that is allowed to log in
//	GET  /emails -> list every registered email
func NewEmailHandler(r *chi.Mux) {
	handler := &emailHandler{
		fileLock: sync.RWMutex{},
	}

	r.Route("/emails", func(r chi.Router) {
		r.Post("/", HandlerFunc(handler.RegisterEmail).ServeHTTP)
	})
}

// registerEmailRequest is the JSON body accepted by RegisterEmail
type registerEmailRequest struct {
	Email string `json:"email"`
}

// RegisterEmail adds an email to the allowlist so that it is allowed to log in
func (h *emailHandler) RegisterEmail(w http.ResponseWriter, r *http.Request) *HandlerError {
	var request registerEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return NewHandlerError("invalid request body, expected a JSON object with an email field", http.StatusBadRequest)
	}

	if strings.TrimSpace(request.Email) == "" {
		return NewHandlerError("email is required", http.StatusBadRequest)
	}

	if !strings.HasSuffix(request.Email, ".gatech.edu") {
		return NewHandlerError("Email must be .gatech.edu email", http.StatusBadRequest)
	}

	h.fileLock.Lock();
	defer h.fileLock.Unlock();

	var allowedEmails []string
	raw, err := os.ReadFile(EMAILS_FILE)
	if err != nil {
		allowedEmails = []string{request.Email}
		os.WriteFile(EMAILS_FILE, []byte("[\"" + request.Email + "\"]"), 0644)
		w.WriteHeader(http.StatusCreated)
		return nil
	}
	
	err = json.Unmarshal(raw, &allowedEmails)
	if err != nil {
		panic(err)
	}
	allowedEmails = append(allowedEmails, request.Email)
	data, err := json.MarshalIndent(allowedEmails, "", "  ")
	if err != nil {
		panic(err)
	}
	os.WriteFile(EMAILS_FILE, data, 0644)
	w.WriteHeader(http.StatusCreated)

	return nil
}
