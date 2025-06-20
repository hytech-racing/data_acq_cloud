package background

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
	"github.com/stretchr/testify/assert"
)

// mockProcessor is a simple implementation of FileJobProcessor that just sets status to completed.
type mockProcessor struct {
	shouldFail bool
}

func (m *mockProcessor) Process(fp *FileProcessor, job *FileJob) error {
	fp.setCurrentlyProcessing(true)
	defer fp.setCurrentlyProcessing(false)

	if m.shouldFail {
		return io.ErrUnexpectedEOF
	}
	fp.updateJobStatus(job, StatusCompleted)
	return nil
}

func TestFileProcessor_EnqueueAndProcessFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create mock dbClient and s3Repository (or pass nil if unused in test)
	fp, err := NewFileProcessor(tempDir, 1024*1024, &database.DatabaseClient{}, &s3.S3Repository{})
	assert.NoError(t, err)
	assert.NotNil(t, fp)

	// Create a temporary file in memory
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "testfile.txt")
	assert.NoError(t, err)

	_, err = part.Write([]byte("test file content"))
	assert.NoError(t, err)
	writer.Close()

	// Parse the file into a multipart.FileHeader
	reader := multipart.NewReader(&buf, writer.Boundary())
	form, err := reader.ReadForm(1024)
	assert.NoError(t, err)
	defer form.RemoveAll()

	fileHeader := form.File["file"][0]

	// Enqueue job
	job, err := fp.EnqueueFile(fileHeader, &mockProcessor{})
	assert.NoError(t, err)
	assert.NotNil(t, job)
	assert.Equal(t, StatusPending, job.Status)

	// Start processing
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fp.Start(ctx)
	defer fp.Stop()

	// Give time for job processing (not ideal but works for test simplicity)
	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, StatusCompleted, job.Status)
	assert.FileExists(t, job.FilePath)
}

func TestFileProcessor_FailedJob(t *testing.T) {
	tempDir := t.TempDir()
	fp, err := NewFileProcessor(tempDir, 1024*1024, &database.DatabaseClient{}, &s3.S3Repository{})
	assert.NoError(t, err)

	// Create fake multipart file
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "fail.txt")
	part.Write([]byte("fail me"))
	writer.Close()

	reader := multipart.NewReader(&buf, writer.Boundary())
	form, err := reader.ReadForm(1024)
	assert.NoError(t, err)
	defer form.RemoveAll()

	fileHeader := form.File["file"][0]

	job, err := fp.EnqueueFile(fileHeader, &mockProcessor{shouldFail: true})
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fp.Start(ctx)
	defer fp.Stop()

	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, StatusFailed, job.Status)
}

func TestFileProcessor_SizeTracking(t *testing.T) {
	tempDir := t.TempDir()
	fp, err := NewFileProcessor(tempDir, 1024*1024, &database.DatabaseClient{}, &s3.S3Repository{})
	assert.NoError(t, err)

	assert.Equal(t, int64(0), fp.TotalSize.Load())

	// Create dummy file
	dummyFile := filepath.Join(tempDir, "dummy.txt")
	content := []byte("dummy data")
	err = os.WriteFile(dummyFile, content, 0o644)
	assert.NoError(t, err)

	fp.TotalSize.Add(int64(len(content)))
	assert.Equal(t, int64(len(content)), fp.TotalSize.Load())
}
