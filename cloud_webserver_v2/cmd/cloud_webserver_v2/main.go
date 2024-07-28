package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	handler "github.com/hytech-racing/cloud-webserver-v2/internal/delivery/http"
)

func main() {
	router := chi.NewRouter()

	// Simple middleware stack
	// r.Use(httplog.RequestLogger(logger))
	router.Use(middleware.Logger)
	router.Use(middleware.Heartbeat("/ping"))
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)

	// Set a timeout value on the request context (ctx), that will signal
	// through ctx.Done() that the request has timed out and further
	// processing should be stopped.
	router.Use(middleware.Timeout(2 * time.Minute))

	router.Mount("/api/v2", router)
	// HTTP handlers
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hytech Data Acquisition and Operations Cloud Webserver"))
	})

	handler.NewMcapHandler(router)

	http.ListenAndServe(":8080", router)
}
