package http

import (
	"encoding/json"
	"net/http"
)

// HandlerFunc serves as a wrapper to allow for direct error returning from normal chi handler methods.
// It doesn't do anything too special and is a nice to have.
// It allows handlers to throw errors with set status codes and mesasges in a consistent and convenient fashion,
// and makes it clear that a thrown error early returns from the handler function.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) *HandlerError

type HandlerError struct {
	Message    string
	StatusCode int
}

func ResponseError(message string, code int) HandlerError {
	return HandlerError{
		Message:    message,
		StatusCode: code,
	}
}

func NewHandlerError(message string, code int) *HandlerError {
	return &HandlerError{
		Message:    message,
		StatusCode: code,
	}
}

func (fn HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}()

	if handlerError := fn(w, r); handlerError != nil {
		handleHTTPError(w, *handlerError)
	}
}

func handleHTTPError(w http.ResponseWriter, err HandlerError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.StatusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":    make([]interface{}, 0),
		"message": err.Message,
	})
}
