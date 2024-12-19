package background

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database"

	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
)

// The current status of a file processor job is one of these statuses
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

// A FileJobProcessor serves as an interface to wrap a Process function used by a FileJob.
// The Process function contains logic to execute a FileJob.
// A job uses the Process function to perform its task.
type FileJobProcessor interface {
	Process(fp *FileProcessor, job *FileJob) error
}

// A FileProcessor handles FileJobs in a queue manner.
// Jobs are currently queued from file uploads, but can be queued for more (in the future).
// FileJobs are processed one by one and the internal logic for handling a Job lives within the FileJob struct.
type FileProcessor struct {
	// dbClient is the client the FileProcessor uses to accesses the database
	dbClient *database.DatabaseClient

	// s3Repository is the client the FileProcessor uses to accesses S3
	s3Repository *s3.S3Repository

	// fileQueueChan is the queue of jobs the FileProcessor reads from and adds to
	fileQueueChan chan *FileJob

	// This channel signals if the FileProcessor needs to stop
	// for whatever reason (mainly to handle a graceful shutdown)
	stopChan chan bool

	// The directory for where the files in FileProcessor live
	directory string

	// The estimated size of the stored FileProcessor files controlled by the middleware
	// We use this so the middleware has an estimate of what our current file capacity is
	MiddlewareEstimatedSize atomic.Int64

	// The actual size of the stored FileProcessor files controlled by the FileProcessor
	// We use this in combination to the middleware estimated size because at certain points of
	// file processing (for example if a FileProcessor is handling multiple mile uploads at once)
	// the size being used at the middleware is not accurate when purely basing it off of the TotalSize
	// Every once in a while, the TotalSize and MiddlewareEstimatedSize are sycned
	TotalSize atomic.Int64

	// processingWg is a WaitGroup used to make sure we complete the last task before gracefuly exiting
	processingWg sync.WaitGroup

	// mutex used to ensure we make FileProcessor functions thread safe
	mu sync.RWMutex

	// maxTotalSize is the total capacity of files we can hold in queue
	maxTotalSize int64

	// activelyProcessing is used to show whether we are actively processing a FileJob
	activelyProcessing bool
}

// FileJob contians all the logic and metadata for completing a job related to files.
// FileJobs operate on a file saved to the server file system.
// Jobs are independent of each other.
type FileJob struct {
	// Processor contians all the logic required to execute a job start to finish
	Processor FileJobProcessor

	// CreatedAt is the time the job was created
	CreatedAt time.Time

	// UpdatedAt is the time the job was updated
	UpdatedAt time.Time

	// The date attatched to a file job
	// It could be the time of upload or date in the metadata of a MCAP
	Date time.Time

	// ID of the job
	ID string

	// Filename is the name of the uploaded file
	Filename string

	// Status is the current status of the job
	// The status is set to one of the consts Status... consts
	Status string

	// FilePath is the absolute path of where the file is
	FilePath string

	// FileDir is the directory of where the file lives
	FileDir string

	// Size is the size of the file in bytes
	Size int64
}

func NewFileProcessor(uploadDir string, maxTotalSize int64, dbClient *database.DatabaseClient, s3Repository *s3.S3Repository) (*FileProcessor, error) {
	err := os.MkdirAll(uploadDir, 0o755)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %v", err)
	}

	fp := &FileProcessor{
		directory:     uploadDir,
		fileQueueChan: make(chan *FileJob, 100),
		stopChan:      make(chan bool),
		processingWg:  sync.WaitGroup{},
		mu:            sync.RWMutex{},
		maxTotalSize:  maxTotalSize,
		dbClient:      dbClient,
		s3Repository:  s3Repository,
	}

	var totalSize int64
	err = filepath.Walk(uploadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	fp.TotalSize.Store(totalSize)
	return fp, nil
}

func (fp *FileProcessor) QueueFile(fileHeader *multipart.FileHeader, processor FileJobProcessor) (*FileJob, error) {
	src, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	id := fmt.Sprintf("job_%d", time.Now().UnixNano())
	job := &FileJob{
		ID:        id,
		Filename:  fileHeader.Filename,
		Size:      fileHeader.Size,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		FilePath:  filepath.Join(fp.directory, fmt.Sprintf("%s_%s", id, fileHeader.Filename)),
		FileDir:   fp.directory,
		Date:      time.Now(), // TODO: Change to date gotten from MCAP
		Processor: processor,
	}

	dst, err := os.Create(job.FilePath)
	if err != nil {
		return nil, err
	}
	defer dst.Close()

	if _, err = io.Copy(dst, src); err != nil {
		os.Remove(job.FilePath)
		return nil, err
	}

	fp.TotalSize.Add(job.Size)
	log.Printf("job put in queue, %v", job.ID)
	fp.fileQueueChan <- job

	return job, nil
}

func (fp *FileProcessor) jobQueueListener(ctx context.Context) {
	defer fp.processingWg.Done()

	for {
		// Ensures that only 1 file is being processed at a time (to save resources)
		// And that a file currently being processed tries to finish
		if fp.activelyProcessing {
			time.Sleep(5 * time.Second)
		}
		select {
		case <-ctx.Done():
			return
		case <-fp.stopChan:
			return
		case job := <-fp.fileQueueChan:
			if err := job.Processor.Process(fp, job); err != nil {
				log.Printf("Failed to process file %s: %v", job.Filename, err)
				fp.updateJobStatus(job, StatusFailed)
				// TODO: Add job status to database
			}
		}
	}
}

func (fp *FileProcessor) updateJobStatus(job *FileJob, status string) {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	job.Status = status
	job.UpdatedAt = time.Now()
}

func (fp *FileProcessor) setCurrentlyProcessing(flag bool) {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	fp.activelyProcessing = flag
}

func (fp *FileProcessor) Start(ctx context.Context) {
	fp.processingWg.Add(1)
	go fp.jobQueueListener(ctx)
}

func (fp *FileProcessor) Stop() {
	close(fp.stopChan)
	fp.processingWg.Wait()
}

func (fp *FileProcessor) MaxTotalSize() int64 {
	return fp.maxTotalSize
}

func (fp *FileProcessor) syncTotalSize() {
	for {
		time.Sleep(1 * time.Minute)
		fp.MiddlewareEstimatedSize.CompareAndSwap(fp.MiddlewareEstimatedSize.Load(), fp.TotalSize.Load())
	}
}
