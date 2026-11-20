package background

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
)

type McapStatus string

// The current status of a file processor job is one of these statuses
const (
	StatusPending    McapStatus = "pending"
	StatusProcessing McapStatus = "processing"
	StatusCompleted  McapStatus = "completed"
	StatusFailed     McapStatus = "failed"
)

var (
	ErrInsufficientCapacity = errors.New("not enough file capacity to stage the run's mcap")
	ErrConversionInProgress = errors.New("a conversion is already in progress for this run")
)

// fileProcessorWorkerCount is greater than one so a job can download or upload while another converts.
const fileProcessorWorkerCount = 3

// A FileJobProcessor serves as an interface to wrap a Process function used by a FileJob.
// The Process function contains logic to execute a FileJob.
// A job uses the Process function to perform its task.
type FileJobProcessor interface {
	ProcessFileJob(fp *FileProcessor, job *FileJob) error
}

// A FileProcessor handles FileJobs in a queue manner.
// Jobs are currently queued from file uploads, but can be queued for more (in the future).
// Workers run several jobs at once so a job can download or upload while another converts,
// but the conversion itself is serialized. The internal logic for handling a Job lives
// within the FileJob struct.
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

	// Keeps track of uploads that are still in-progress, keyed by job ID so two jobs for
	// the same file name do not share a marker.
	pendingMcapFileUploads map[string]string

	// Keeps track of runs with a re-conversion queued or running, so the same run is
	// never converted twice at once.
	pendingRerunConversions map[primitive.ObjectID]struct{}

	// processingWg is a WaitGroup used to make sure we complete the last task before gracefuly exiting
	processingWg sync.WaitGroup

	// mutex used to ensure we make FileProcessor functions thread safe
	mu sync.RWMutex

	// maxTotalSize is the total capacity of files we can hold in queue
	maxTotalSize int64

	// mcapStatusBroadcaster broadcasts updates about in-progress MCAP uploads to SSE subscribers.
	mcapStatusBroadcaster *McapStatusBroadcaster
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
	Status McapStatus

	// FilePath is the absolute path of where the file is
	FilePath string

	// FileDir is the directory of where the file lives
	FileDir string

	// SourceS3Key is the key the client uploaded the file to. The processor stages the
	// object to FilePath before running the job, reusing the archived copy when one is
	// complete; it is never re-uploaded because it already lives in the bucket.
	SourceS3Key string

	// Size is the size of the file in bytes
	Size int64

	// ReusedLocalCopy reports that FilePath points at a file this job did not create, so
	// the job must leave it in place for later jobs to reuse.
	ReusedLocalCopy bool

	Hidden bool
}

// NewFileProcessor creates a new File Processor struct instance and populates is with
// the pre-existing file data if such information exists.
func NewFileProcessor(uploadDir string, maxTotalSize int64, dbClient *database.DatabaseClient, s3Repository *s3.S3Repository) (*FileProcessor, error) {
	err := os.MkdirAll(uploadDir, 0o755)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %v", err)
	}

	fp := &FileProcessor{
		directory:               uploadDir,
		fileQueueChan:           make(chan *FileJob, 100),
		stopChan:                make(chan bool),
		processingWg:            sync.WaitGroup{},
		mu:                      sync.RWMutex{},
		maxTotalSize:            maxTotalSize,
		dbClient:                dbClient,
		s3Repository:            s3Repository,
		mcapStatusBroadcaster:   NewMcapStatusBroadcaster(),
		pendingMcapFileUploads:  make(map[string]string),
		pendingRerunConversions: make(map[primitive.ObjectID]struct{}),
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
	go fp.syncTotalSize()

	return fp, nil
}

// EnqueueS3Object creates a FileJob for an object the client uploaded directly to S3.
// The object is downloaded into the upload directory by the queue worker, so the HTTP
// request that registers the upload returns immediately no matter how large the file is.
func (fp *FileProcessor) EnqueueS3Object(objectKey, filename string, size int64, processor FileJobProcessor, hidden bool) (*FileJob, error) {
	if !fp.HasCapacityFor(size) {
		return nil, ErrInsufficientCapacity
	}

	job := fp.newFileJob(objectKey, filename, size, processor)
	job.Hidden = hidden

	log.Printf("job put in queue for direct upload %v, %v", objectKey, job.ID)
	fp.queueJob(job)

	return job, nil
}

// EnqueueRerunConversion queues a job that regenerates a run's converted files from the
// MCAP file the run already stores in S3. The run is marked as converting as soon as the
// job is queued, so a second request gets ErrConversionInProgress instead of converting
// the same run twice.
func (fp *FileProcessor) EnqueueRerunConversion(vehicleRunID primitive.ObjectID, mcapFile models.FileModel, size int64) (*FileJob, error) {
	if !fp.HasCapacityFor(size) {
		return nil, ErrInsufficientCapacity
	}

	fp.mu.Lock()
	if _, exists := fp.pendingRerunConversions[vehicleRunID]; exists {
		fp.mu.Unlock()
		return nil, ErrConversionInProgress
	}
	fp.pendingRerunConversions[vehicleRunID] = struct{}{}
	fp.mu.Unlock()

	fileName := mcapFile.FileName
	if fileName == "" {
		// Older database entries may not have stored a file name.
		fileName = filepath.Base(mcapFile.FilePath)
	}

	job := fp.newFileJob(mcapFile.FilePath, fileName, size, &PostProcessMCAPUploadJob{RerunOf: &vehicleRunID})

	log.Printf("re-conversion queued for vehicle run %v, %v", vehicleRunID.Hex(), job.ID)
	fp.queueJob(job)

	return job, nil
}

// newFileJob builds the state every queued job shares. Hidden is set by the caller.
func (fp *FileProcessor) newFileJob(sourceS3Key, filename string, size int64, processor FileJobProcessor) *FileJob {
	id := fmt.Sprintf("job_%s", primitive.NewObjectID().Hex())
	now := time.Now()

	return &FileJob{
		ID:          id,
		Filename:    filename,
		Size:        size,
		Status:      StatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
		FilePath:    filepath.Join(fp.directory, fmt.Sprintf("%s_%s", id, filename)),
		FileDir:     fp.directory,
		SourceS3Key: sourceS3Key,
		Date:        now, // TODO: Change to date gotten from MCAP
		Processor:   processor,
	}
}

// queueJob counts the file the job will stage against the processor's limits and hands
// the job to the worker. finishJob reverses the space once the job is done.
func (fp *FileProcessor) queueJob(job *FileJob) {
	fp.TotalSize.Add(job.Size)
	fp.MiddlewareEstimatedSize.Add(job.Size)
	fp.fileQueueChan <- job
}

// HasCapacityFor reports whether the file processor can stage another size bytes. It
// looks at both the actual size of the files in the upload directory and the
// middleware's estimate, which includes uploads that are still in flight.
func (fp *FileProcessor) HasCapacityFor(size int64) bool {
	currentSize := fp.TotalSize.Load()
	if estimated := fp.MiddlewareEstimatedSize.Load(); estimated > currentSize {
		currentSize = estimated
	}
	return currentSize+size <= fp.maxTotalSize
}

func (fp *FileProcessor) downloadAndStageMCAP(ctx context.Context, job *FileJob) (*float64, string, error) {
	if hash, durationSeconds, ok := pullFromArchive(job); ok {
		return durationSeconds, hash, nil
	}

	file, err := os.Create(job.FilePath)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	downloaded, err := fp.s3Repository.DownloadObject(ctx, job.SourceS3Key, file)
	if err != nil {
		return nil, "", err
	}

	fileHash, err := hashStagedFile(file, downloaded, job.Size)
	if err != nil {
		return nil, "", err
	}

	durationSeconds, err := utils.McapDurationSeconds(io.NewSectionReader(file, 0, job.Size))
	if err != nil {
		log.Printf("could not read MCAP duration for %v: %v", job.FilePath, err)
	}

	return durationSeconds, fileHash, nil
}

// hashStagedFile returns the SHA-256 of a staged file and rejects a download whose byte
// count does not match the size the client reported.
func hashStagedFile(file *os.File, downloaded, expected int64) (string, error) {
	if downloaded != expected {
		return "", fmt.Errorf("staged file %v has %d bytes, expected %d", file.Name(), downloaded, expected)
	}
	return utils.CreateFileHash(file)
}

// pullFromArchive points the job at the MCAP a previous conversion archived in the run
// metadata volume, returning its hash and duration.
func pullFromArchive(job *FileJob) (string, *float64, bool) {
	localPath := filepath.Join(RUN_METADATA_DIR, job.SourceS3Key)

	info, err := os.Stat(localPath)
	if err != nil || info.Size() != job.Size {
		return "", nil, false
	}

	file, err := os.Open(localPath)
	if err != nil {
		return "", nil, false
	}
	defer file.Close()

	hash, err := utils.CreateFileHash(file)
	if err != nil {
		return "", nil, false
	}

	durationSeconds, err := utils.McapDurationSeconds(io.NewSectionReader(file, 0, job.Size))
	if err != nil {
		log.Printf("could not read MCAP duration for %v: %v", localPath, err)
	}

	job.FilePath = localPath
	job.ReusedLocalCopy = true
	log.Printf("reusing archived copy %v for job %v", localPath, job.ID)

	return hash, durationSeconds, true
}

// finishJob removes the job's staged file and releases the space it occupied. It runs
// exactly once per processed job, whether the job succeeded or failed.
func (fp *FileProcessor) finishJob(job *FileJob) {
	// A reused archived copy is shared with later jobs, so only a file this job staged is removed.
	if !job.ReusedLocalCopy {
		if err := os.Remove(job.FilePath); err != nil && !os.IsNotExist(err) {
			log.Printf("failed to remove processed file %v: %v", job.FilePath, err)
		}
	}
	fp.TotalSize.Add(-job.Size)
	fp.MiddlewareEstimatedSize.Add(-job.Size)

	if rerun, ok := job.Processor.(*PostProcessMCAPUploadJob); ok && rerun.RerunOf != nil {
		fp.mu.Lock()
		delete(fp.pendingRerunConversions, *rerun.RerunOf)
		fp.mu.Unlock()
	}
}

// jobWorker processes queued jobs until the processor stops.
func (fp *FileProcessor) jobWorker(ctx context.Context) {
	defer fp.processingWg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-fp.stopChan:
			return
		case job := <-fp.fileQueueChan:
			if err := job.Processor.ProcessFileJob(fp, job); err != nil {
				fp.broadcastFileUploadError(job, err)
			}
		}
	}
}

func (fp *FileProcessor) broadcastFileUploadStart(job *FileJob) {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	log.Printf("MCAP File %v is being uploaded.", job.Filename)
	job.Status = StatusProcessing
	job.UpdatedAt = time.Now()
	fp.pendingMcapFileUploads[job.ID] = job.Filename
	fp.mcapStatusBroadcaster.Broadcast(McapStatusEvent{
		Status: StatusProcessing,
		Name:   job.Filename,
	})
}

func (fp *FileProcessor) broadcastFileUploadError(job *FileJob, error error) {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	log.Printf("MCAP File %v did not upload. Error: %v", job.Filename, error)
	job.Status = StatusFailed
	job.UpdatedAt = time.Now()
	delete(fp.pendingMcapFileUploads, job.ID)
	fp.mcapStatusBroadcaster.Broadcast(McapStatusEvent{
		Status: StatusFailed,
		Name:   job.Filename,
		Error:  error.Error(),
	})
}

func (fp *FileProcessor) broadcastFileUploadEnd(job *FileJob, ctx context.Context, vehicleRunModel *models.VehicleRunModel) {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	log.Printf("File %v is marked as done", job.Filename)
	job.Status = StatusCompleted
	job.UpdatedAt = time.Now()
	delete(fp.pendingMcapFileUploads, job.ID)
	fp.mcapStatusBroadcaster.Broadcast(McapStatusEvent{
		Status: StatusCompleted,
		Data:   models.VehicleRunSerialize(ctx, fp.s3Repository, *vehicleRunModel),
	})
}

func (fp *FileProcessor) GetPendingMcapUploads() []string {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	fileUploads := make([]string, 0, len(fp.pendingMcapFileUploads))
	for _, fileUpload := range fp.pendingMcapFileUploads {
		fileUploads = append(fileUploads, fileUpload)
	}
	return fileUploads
}

func (fp *FileProcessor) SubscribeToMcapStatus() chan []byte {
	return fp.mcapStatusBroadcaster.Subscribe()
}

func (fp *FileProcessor) UnsubscribeFromMcapStatus(subscriber chan []byte) {
	fp.mcapStatusBroadcaster.Unsubscribe(subscriber)
}

// Start starts the FileProcessor's workers.
func (fp *FileProcessor) Start(ctx context.Context) {
	for i := 0; i < fileProcessorWorkerCount; i++ {
		fp.processingWg.Add(1)
		go fp.jobWorker(ctx)
	}
}

// Stop stops the file processor and waits for up to fileProcessorWorkerCount in-flight jobs.
func (fp *FileProcessor) Stop() {
	close(fp.stopChan)
	fp.processingWg.Wait()
}

// MaxTotalSize returns the max size allocated to the FileProcessor
func (fp *FileProcessor) MaxTotalSize() int64 {
	return fp.maxTotalSize
}

// syncTotalSize syncs the queued file size between the middleware's estimate and the actual size
func (fp *FileProcessor) syncTotalSize() {
	for {
		time.Sleep(1 * time.Minute)
		fp.MiddlewareEstimatedSize.CompareAndSwap(fp.MiddlewareEstimatedSize.Load(), fp.TotalSize.Load())
	}
}
