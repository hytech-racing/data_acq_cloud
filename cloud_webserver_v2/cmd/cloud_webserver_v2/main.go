package main

import (
	"context"
	_ "embed"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/hytech-racing/cloud-webserver-v2/internal/background"
	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
	handler "github.com/hytech-racing/cloud-webserver-v2/internal/delivery/http"
	"github.com/hytech-racing/cloud-webserver-v2/internal/logging"
	hytech_middleware "github.com/hytech-racing/cloud-webserver-v2/internal/middleware"
	"github.com/hytech-racing/cloud-webserver-v2/internal/mps"
	proto_sync "github.com/hytech-racing/cloud-webserver-v2/internal/proto_sync"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
	"github.com/joho/godotenv"
)

/* TODO:
   - [x] Dynamically decode protobuf messages
   - [x] Add AWS S3 Support
   - [x] Add subscriber to plot Lat/Lon data
   - [x] Add subscriber to create raw and intepolated MATLAB files
   - [x] Add MATLAB Writing support with Python (for now as a quick fix, eventually we should figure out a better way)
   - [x] Add MongoDB Support
   - [x] Create repositories to make our database interactions clean, scalable, and extendable
   - [ ] Add better error handling into the server -> we want to gracefully handle errors and continue on
   - [ ] Add better and more informative logging
   - [ ] Add tests for all components of the server (I want to check out testcontainers it seems really nice)
*/

//go:embed buzz.txt
var buzz string

func main() {
	println(buzz)

	// Initialize Crash Logger Globally
	logging.InitLogger(50)
	customLogger := logging.GetLogger()
	defer customLogger.RecoverAndLogPanic()

	log.Println("Server starting...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// load .env file
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file %s", err)
	}
	log.Println("Loaded .env file...")

	mpsURI := os.Getenv("MATLAB_URI")
	mpsClient := mps.NewMatlabClient(mpsURI)
	mpsClient.PollForResults()

	// Setup database our database connection
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		log.Fatal("could not get mongodb uri environment variable")
	}
	dbClient, err := database.NewDatabaseClient(ctx, uri)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Connected to database...")

	// Setup aws s3 connection
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		log.Fatal("could not get aws region environment variable")
	}

	awsBucket := os.Getenv("AWS_S3_RUN_BUCKET")
	if awsBucket == "" {
		log.Fatal("could not get aws bucket environment variable")
	}

	awsAccessKey := os.Getenv("AWS_ACCESS_KEY")
	if awsAccessKey == "" {
		log.Fatal("could not get aws access key environment variable")
	}

	awsSecretKey := os.Getenv("AWS_SECRET_KEY")
	if awsSecretKey == "" {
		log.Fatal("could not get aws secret key environment variable")
	}

	// We are creating one connection to AWS S3 and passing that around to all the methods to save resources
	s3Repository := s3.NewS3Session(awsAccessKey, awsSecretKey, awsRegion, awsBucket)
	log.Println("Started S3 session...")

	// Adding HT_Proto Listener...
	proto_listener := proto_sync.Initializer(ctx, s3Repository)

	// Create file fileProcessor with 10GB limit
	fileProcessor, err := background.NewFileProcessor(
		"./uploads",
		10*1024*1024*1024, // 10GB
		dbClient,
		s3Repository,
	)
	if err != nil {
		log.Fatal(err)
	}

	fileProcessor.Start(ctx)

	fileUploadMiddleware := hytech_middleware.FileUploadMiddleware{
		FileProcessor: fileProcessor,
	}

	router := chi.NewRouter()

	// CORS Setup
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://hytech-racing.github.io", "http://localhost:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	}))

	// Simple middleware stack
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
	log.Println("Ready to accept requests!")
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("HyTech Data Acquisition and Operations Cloud Webserver"))
	})

	handler.NewMcapHandler(router, s3Repository, dbClient, fileProcessor, &fileUploadMiddleware, mpsClient)
	handler.NewUploadHandler(router, dbClient, fileProcessor)
	handler.NewDocumentationHandler(router, s3Repository)

	// Graceful shutdown: listen for interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		// Wait for a signal, then gracefully shut down
		<-quit
		println()
		log.Println("Shutting down server...")

		log.Println("Waiting for file processor to finish...")
		fileProcessor.Stop()

		proto_listener.Stop()

		// Gracefully disconnect from MongoDB
		mongoShutdownCtx, mongoShutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer mongoShutdownCancel()

		if err := dbClient.Disonnect(mongoShutdownCtx); err != nil {
			log.Println("Error while disconnecting MongoDB:", err)
		} else {
			log.Println("Disconnected from MongoDB")
		}

		os.Exit(0)
	}()

	http.ListenAndServe(":8080", router)
}
