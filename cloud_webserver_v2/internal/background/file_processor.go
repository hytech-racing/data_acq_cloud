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

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

type FileJobProcessor interface {
	Process(fp *FileProcessor, job *FileJob) error
}

type FileProcessor struct {
	uploadDir               string
	queueChan               chan *FileJob
	stopChan                chan bool
	processingWg            sync.WaitGroup
	activelyProcessing      bool
	mu                      sync.RWMutex
	MiddlewareEstimatedSize atomic.Int64
	TotalSize               atomic.Int64
	maxTotalSize            int64
	dbClient                *database.DatabaseClient
	s3Repository            *s3.S3Repository
}

type FileJob struct {
	ID        string
	Filename  string
	Size      int64
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
	FilePath  string
	FileDir   string
	Date      time.Time
	Processor FileJobProcessor
}

func NewFileProcessor(uploadDir string, maxTotalSize int64, dbClient *database.DatabaseClient, s3Repository *s3.S3Repository) (*FileProcessor, error) {
	err := os.MkdirAll(uploadDir, 0o755)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %v", err)
	}

	fp := &FileProcessor{
		uploadDir:    uploadDir,
		queueChan:    make(chan *FileJob, 100),
		stopChan:     make(chan bool),
		processingWg: sync.WaitGroup{},
		mu:           sync.RWMutex{},
		maxTotalSize: maxTotalSize,
		dbClient:     dbClient,
		s3Repository: s3Repository,
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
		FilePath:  filepath.Join(fp.uploadDir, fmt.Sprintf("%s_%s", id, fileHeader.Filename)),
		FileDir:   fp.uploadDir,
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
	fp.queueChan <- job

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
		case job := <-fp.queueChan:
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
