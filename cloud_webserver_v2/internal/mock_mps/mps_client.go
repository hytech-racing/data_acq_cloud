package mock_mps

// import (
// 	"context"
// 	"log"
// 	"net/http"
// 	"os"
// 	"time"
//
// 	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
// 	"go.mongodb.org/mongo-driver/bson/primitive"
// )
//
// // h5FileDirectory is the directory where the .h5 files are stored locally, acts as s3 cache
// const h5FileDirectory = "/data/run_metadata/"
//
// // mpsInstanceDirectory is the path of the MPS instance directory
// // /mps_data is the mount point of the mps_data Docker volume on this container
// const mpsInstanceDirectory = "/mps_data/mps_workspace/Instances/mps_2"
//
// // matlabJobState represents the state of a MATLAB job
// // View https://www.mathworks.com/help/mps/restfuljson/getstateinformation.html for more information
// type matlabJobState string
//
// const (
// 	READING    matlabJobState = "READING"
// 	IN_QUEUE   matlabJobState = "IN_QUEUE"
// 	PROCESSING matlabJobState = "PROCESSING"
// 	READY      matlabJobState = "READY"
// 	ERROR      matlabJobState = "ERROR"
// 	CANCELLED  matlabJobState = "CANCELLED"
// )
//
// // mpsJob represents a MATLAB job submitted to the MPS
// // It includes important information about the job
// type mpsJob struct {
// 	// ID of the MCAP associated with the job
// 	mcapId primitive.ObjectID
//
// 	// URL of the job status
// 	jobId string
//
// 	// Package version of the MATLAB function
// 	packageVersion string
//
// 	// Function name of the MATLAB function
// 	functionName string
// }
//
// // Represents HTTP response of an MPS async job request
// // View https://www.mathworks.com/help/mps/restfuljson/postasynchronousrequest.html for more information
// type matlabJobResponse struct {
// 	// ID of a particular request
// 	ID string `json:"id"`
//
// 	// URI of particular request
// 	Self string `json:"self"`
//
// 	// URI used to view a collection of requests on the server
// 	Up string `json:"up"`
//
// 	// Number indicating when a request represented by self was last modified
// 	LastModifiedSeq int `json:"lastModifiedSeq"`
//
// 	// State of a request
// 	State matlabJobState `json:"state"`
//
// 	// Client id or name that was specified as a query parameter while initiating a request
// 	Client string `json:"client"`
// }
//
// // MatlabClient is a client for the MATLAB Production Server (MPS)
// // It handles the submission of MATLAB jobs and processing of results
// type MatlabClient struct {
// 	// URI of the MPS server
// 	mpsBaseUrl string
//
// 	// Contains the ids of all the jobs submitted and are being processed/queued
// 	jobsProcessing []mpsJob
// 	dbClient       *database.DatabaseClient
//
// 	// Duration to wait between polling for job results
// 	pollDuration time.Duration
// }
//
// func NewMatlabClient(dbClient *database.DatabaseClient, _ string, pollDuration time.Duration) *MatlabClient {
// 	return &MatlabClient{
// 		jobsProcessing: []mpsJob{},
// 		dbClient:       dbClient,
// 		pollDuration:   pollDuration,
// 	}
// }
//
// // Submits a new synchronous job to the MPS.
// // The MPS client will save the job id and wait for the result and process it in the background
// // View https://www.mathworks.com/help/mps/restfuljson/postasynchronousrequest.html for more information
// func (m *MatlabClient) SubmitMatlabJob(ctx context.Context, s3Repo *s3.S3Repository, mcapId string, packageName string, functionName string) {
// 	log.Println("submitting matlab job")
//
// 	primitiveId, err := primitive.ObjectIDFromHex(mcapId)
// 	if err != nil {
// 		log.Fatalf("error converting mcapId to primitive.ObjectID: %v", err)
// 	}
//
// 	model, err := m.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, primitiveId)
// 	if err != nil {
// 		log.Fatalf("error getting vehicle run model: %v", err)
// 	}
//
// 	// ensure that the .h5 file exists on file system in h5FileDirectory
// 	h5FilePath := model.MatFiles[0].FilePath
// 	localFilePath := h5FileDirectory + h5FilePath
// 	if _, err := os.Stat(localFilePath); os.IsNotExist(err) {
// 		err = s3Repo.DownloadObject(ctx, model.MatFiles[0].AwsBucket, h5FilePath, localFilePath)
// 		if err != nil {
// 			log.Fatalf("error downloading file from s3: %v", err)
// 		}
// 	}
// }
