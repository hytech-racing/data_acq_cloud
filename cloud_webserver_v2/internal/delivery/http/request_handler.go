package http

import (
	"encoding/json"
	"net/http"
)

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
	// if httpErr, ok := err.(*); ok {
	//     // If it's an HTTPError, send structured JSON
	//     w.Header().Set("Content-Type", "application/json")
	//     w.WriteHeader(httpErr.Code)
	//     json.NewEncoder(w).Encode(map[string]interface{}{
	//         "error":   httpErr.Message,
	//         "status":  httpErr.Code,
	//     })
	// } else {
	//     // For unexpected errors, send a generic 500 response
	//     w.Header().Set("Content-Type", "application/json")
	//     w.WriteHeader(http.StatusInternalServerError)
	//     json.NewEncoder(w).Encode(map[string]interface{}{
	//         "error":   "Internal server error",
	//         "status":  http.StatusInternalServerError,
	//     })
	// }
}
