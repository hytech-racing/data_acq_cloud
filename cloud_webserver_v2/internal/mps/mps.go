package mps

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

type MatlabClient struct {
	// URI of the MPS server
	mpsBaseUrl string

	// Contains the ids of all the jobs submitted and are being processed/queued
	jobsProcessing []string
}

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
// https://www.mathworks.com/help/mps/restfuljson/postasynchronousrequest.html
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
// https://www.mathworks.com/help/mps/restfuljson/getresultofrequest.html
type matlabJobResult struct {
	// LHS resprents all the results calculated
	// For our purposes, LHS will always be an array with 1 JSON string
	LHS []string `json:"lhs"`
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

func NewMatlabClient(mpsBaseUrl string) *MatlabClient {
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
		jobsProcessing: []string{},
	}
}

// Enables the poll for result loop
func (m *MatlabClient) PollForResults() {
	go m.pollForResults()
}

func (m *MatlabClient) pollForResults() {
	for {
		newJobsProcessing := []string{}
		for _, job := range m.jobsProcessing {
			resp, err := http.Get(m.mpsBaseUrl + job)
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
				m.processResult(job)
				m.deleteMatlabJobResult(job)
			} else {
				newJobsProcessing = append(newJobsProcessing, job)
			}
		}
		m.jobsProcessing = newJobsProcessing
		time.Sleep(10 * time.Second)
	}
}

func (m *MatlabClient) processResult(jobId string) {
	log.Printf("processing result for mps job: %s", jobId)

	resp, err := http.Get(m.mpsBaseUrl + jobId + "/result")
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

	log.Printf("result for mps job %s: %s", jobId, data.LHS[0])
}

// Removes the job as well as the job result from the MPS.
// https://www.mathworks.com/help/mps/restfuljson/deleterequest.html
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
// https://www.mathworks.com/help/mps/restfuljson/postasynchronousrequest.html
func (m *MatlabClient) SubmitMatlabJob(h5FileName string, packageName string, functionName string) {
	log.Println("submitting matlab job")
	payload := newMatlabJobRequestPayload([]string{"/home/hytech/" + h5FileName})
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

	m.jobsProcessing = append(m.jobsProcessing, data.Self)

	log.Printf("matlab job submitted, %s", data.Self)
}
