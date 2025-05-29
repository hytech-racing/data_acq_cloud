package mps

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const h5FileDirectory = "/data/run_metadata/"

// mpsJob represents a MATLAB job submitted to the MPS
// It includes important information about the job
type mpsJob struct {
	// ID of the MCAP associated with the job
	mcapId primitive.ObjectID

	// URL of the job status
	jobId string

	// Package version of the MATLAB function
	packageVersion string

	// Function name of the MATLAB function
	functionName string
}

// MatlabClient is a client for the MATLAB Production Server (MPS)
// It handles the submission of MATLAB jobs and processing of results
type MatlabClient struct {
	// URI of the MPS server
	mpsBaseUrl string

	// Contains the ids of all the jobs submitted and are being processed/queued
	jobsProcessing []mpsJob
	dbClient       *database.DatabaseClient

	// Duration to wait between polling for job results
	pollDuration time.Duration
}

// matlabJobState represents the state of a MATLAB job
// View https://www.mathworks.com/help/mps/restfuljson/getstateinformation.html for more information
type matlabJobState string

const (
	READING    matlabJobState = "READING"
	IN_QUEUE   matlabJobState = "IN_QUEUE"
	PROCESSING matlabJobState = "PROCESSING"
	READY      matlabJobState = "READY"
	ERROR      matlabJobState = "ERROR"
	CANCELLED  matlabJobState = "CANCELLED"
)

// Represents HTTP response of an MPS async job request
// View https://www.mathworks.com/help/mps/restfuljson/postasynchronousrequest.html for more information
type matlabJobResponse struct {
	// ID of a particular request
	ID string `json:"id"`

	// URI of particular request
	Self string `json:"self"`

	// URI used to view a collection of requests on the server
	Up string `json:"up"`

	// Number indicating when a request represented by self was last modified
	LastModifiedSeq int `json:"lastModifiedSeq"`

	// State of a request
	State matlabJobState `json:"state"`

	// Client id or name that was specified as a query parameter while initiating a request
	Client string `json:"client"`
}

// Represents HTTP response of the result of a MPS job request
// View https://www.mathworks.com/help/mps/restfuljson/getresultofrequest.html for more information
type matlabJobResult struct {
	// LHS resprents all the results calculated
	// For our purposes, LHS will always be an array with 1 MpsScriptResult
	LHS []MpsScriptResult `json:"lhs"`
}

// MpsScriptResult represents the schema of the returned result of a MATLAB script
type MpsScriptResult struct {
	// Type can be "mat", "image", or "text"
	Type string `json:"type"`

	// If Type is "mat" or "image", Result will be a path to the file as a string
	// If Type is "text"", Result will be the result as a string
	Result string `json:"result"`
}

// Represents the HTTP request payload of an MPS async job request
// https://www.mathworks.com/help/mps/restfuljson/postasynchronousrequest.html
type matlabJobRequestPayload struct {
	// Number of outputs that the client application is requesting from the deployed MATLAB® function.
	Nargout int `json:"nargout"`

	// Input arguments to the deployed MATLAB function, specified as an array of strings
	// For our purposes, RHS will be an array of 1 string which is the path to an .h5 file
	RHS []string `json:"rhs"`

	// Specify the notation of the MATLAB output response
	// We always use the "small" mode and the "string" nanInfFormat
	OutputFormat struct {
		Mode         string `json:"mode"`
		NanInfFormat string `json:"nanInfFormat"`
	} `json:"outputFormat"`
}

// Creates a new MATLAB job request payload
// rhs represents the arguments passed into the function which should
// always be a single string array which is the filepath to the h5 file
func newMatlabJobRequestPayload(rhs []string) *matlabJobRequestPayload {
	return &matlabJobRequestPayload{
		Nargout: 1,
		RHS:     rhs,
		OutputFormat: struct {
			Mode         string `json:"mode"`
			NanInfFormat string `json:"nanInfFormat"`
		}{
			Mode:         "small",
			NanInfFormat: "string",
		},
	}
}

// Creates a new MATLAB client
func NewMatlabClient(dbClient *database.DatabaseClient, mpsBaseUrl string, pollDuration time.Duration) *MatlabClient {
	resp, err := http.Get(mpsBaseUrl + "/api/health")

	if err != nil {
		log.Panicf("mps client error connecting to %s: %v", mpsBaseUrl, err)
	}

	if resp.StatusCode != 200 {
		log.Panicf("mps client error connecting to %s: %v", mpsBaseUrl, err)
	}

	log.Println("connected to mps")

	return &MatlabClient{
		mpsBaseUrl:     mpsBaseUrl,
		jobsProcessing: []mpsJob{},
		dbClient:       dbClient,
		pollDuration:   pollDuration,
	}
}

// Polls the MPS for the result of a job until it is ready
// Once it's ready, it processes the job result and then deletes it off MPS
func (m *MatlabClient) pollForJobResult(ctx context.Context, mpsJob mpsJob) {
	for {
		resp, err := http.Get(m.mpsBaseUrl + mpsJob.jobId)
		if err != nil {
			log.Fatalf("error getting job status: %v", err)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("error reading response body: %v", err)
		}

		var data matlabJobResponse
		err = json.Unmarshal(body, &data)
		if err != nil {
			log.Fatalf("error unmarshalling response body: %v", err)
		}

		if data.State == READY {
			m.processResult(ctx, mpsJob)
			m.deleteMatlabJobResult(mpsJob.jobId)
		} else {
			time.Sleep(m.pollDuration)
		}

		// TODO: handle other states like errors
	}
}

// Helper function that contains the logic for processing script results from MPS
// Stores the results properly into MongoDB and S3
func (m *MatlabClient) processResult(ctx context.Context, job mpsJob) {
	log.Printf("processing result for mps job: %s", job.jobId)

	resp, err := http.Get(m.mpsBaseUrl + job.jobId + "/result")
	if err != nil {
		log.Fatalf("error getting job result: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatalf("error getting job result: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("error reading response body: %v", err)
	}

	var data matlabJobResult
	err = json.Unmarshal(body, &data)
	if err != nil {
		log.Fatalf("error unmarshalling response body: %v", err)
	}

	scriptResult := data.LHS[0]

	// get current run information from database
	runModel, err := m.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, job.mcapId)
	if err != nil {
		log.Fatalf("could not get vehicle run by id %v, %v", job.mcapId, err)
	}

	// update the model
	// TODO: flesh out the logic more here
	// different things should happen depending on the type of result
	switch scriptResult.Type {
	case "mat":
	case "image":
		// ensure file exists
		// move the file over to local s3 cache and then upload to s3
	case "text":
		runModel.MpsRecord[job.packageVersion][job.functionName] = models.MpsScriptResultModel{
			Type:   scriptResult.Type,
			Result: scriptResult.Result,
		}
	}

	// update the vehicle run in the database
	err = m.dbClient.VehicleRunUseCase().UpdateVehicleRun(ctx, job.mcapId, runModel)
	if err != nil {
		log.Fatalf("could not update vehicle run %v, %v", job.mcapId, err)
	}

	log.Printf("saved result for mps job into mongodb %s: %s", job.jobId, data.LHS[0])
}

// Removes the job as well as the job result from the MPS.
// View https://www.mathworks.com/help/mps/restfuljson/deleterequest.html for more information
func (m *MatlabClient) deleteMatlabJobResult(jobId string) {
	req, err := http.NewRequest("DELETE", m.mpsBaseUrl+jobId, nil)

	if err != nil {
		log.Fatalf("error creating http delete request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		log.Fatalf("error deleting mps job result: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		log.Fatalf("error deleting mps job result: %v", err)
	}

	log.Printf("deleted mps job result %s", jobId)
}

// Submits a new synchronous job to the MPS.
// The MPS client will save the job id and wait for the result and process it in the background
// View https://www.mathworks.com/help/mps/restfuljson/postasynchronousrequest.html for more information
func (m *MatlabClient) SubmitMatlabJob(ctx context.Context, s3Repo *s3.S3Repository, mcapId string, packageName string, functionName string) {
	log.Println("submitting matlab job")

	primitiveId, err := primitive.ObjectIDFromHex(mcapId)
	if err != nil {
		log.Fatalf("error converting mcapId to primitive.ObjectID: %v", err)
	}

	model, err := m.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, primitiveId)
	if err != nil {
		log.Fatalf("error getting vehicle run model: %v", err)
	}

	// ensure that the .h5 file exists on file system in h5FileDirectory
	h5FilePath := model.MatFiles[0].FilePath
	localFilePath := h5FileDirectory + h5FilePath
	if _, err := os.Stat(localFilePath); os.IsNotExist(err) {
		err = s3Repo.DownloadObject(ctx, model.MatFiles[0].AwsBucket, h5FilePath, localFilePath)
		if err != nil {
			log.Fatalf("error downloading file from s3: %v", err)
		}
	}

	payload := newMatlabJobRequestPayload([]string{h5FileDirectory + h5FilePath})
	payloadJson, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("error marshalling payload: %v", err)
	}

	r, err := http.Post(m.mpsBaseUrl+"/"+packageName+"/"+functionName+"?mode=async", "application/json", bytes.NewBuffer(payloadJson))

	if err != nil {
		log.Fatalf("error submitting matlab file: %v", err)
	}

	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Fatalf("error reading response body: %v", err)
	}

	var data matlabJobResponse
	err = json.Unmarshal(body, &data)

	if err != nil {
		log.Fatalf("error unmarshalling response body: %v", err)
	}

	// spawn go routine to poll for result
	go m.pollForJobResult(ctx, mpsJob{
		mcapId:         primitiveId,
		jobId:          data.Self,
		packageVersion: packageName,
		functionName:   functionName,
	})

	log.Printf("matlab job submitted, %s", data.Self)
}
