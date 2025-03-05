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
	mpsBaseUrl     string
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

type matlabJobResponse struct {
	ID              string         `json:"id"`
	Self            string         `json:"self"`
	Up              string         `json:"up"`
	LastModifiedSeq int            `json:"lastModifiedSeq"`
	State           matlabJobState `json:"state"`
	Client          string         `json:"client"`
}

type matlabJobResult struct {
	LHS []struct {
		Mwdata []string `json:"mwdata"`
		Mwsize []int    `json:"mwsize"`
		Mwtype string   `json:"mwtype"`
	} `json:"lhs"`
}

type matlabJobRequestPayload struct {
	Nargout      int      `json:"nargout"`
	RHS          []string `json:"rhs"`
	OutputFormat struct {
		Mode         string `json:"mode"`
		NanInfFormat string `json:"nanInfFormat"`
	} `json:"outputFormat"`
}

func NewMatlabJobRequestPayload(rhs []string) *matlabJobRequestPayload {
	return &matlabJobRequestPayload{
		Nargout: 1,
		RHS:     rhs,
		OutputFormat: struct {
			Mode         string `json:"mode"`
			NanInfFormat string `json:"nanInfFormat"`
		}{
			Mode:         "async",
			NanInfFormat: "text",
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
				m.DeleteMatlabJobResult(job)
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

	log.Printf("result for mps job %s: %s", jobId, data.LHS[0].Mwdata[0])
}

func (m *MatlabClient) DeleteMatlabJobResult(jobId string) {
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

func (m *MatlabClient) SubmitMatlabJob(h5FileName string, packageName string, functionName string) {
	log.Println("submitting matlab job")
	payload := NewMatlabJobRequestPayload([]string{"/home/hytech/" + h5FileName})
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
