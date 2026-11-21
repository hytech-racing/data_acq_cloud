package background

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/foxglove/mcap/go/mcap"
	"github.com/hytech-racing/cloud-webserver-v2/internal/messaging"
	"gonum.org/v1/hdf5"
)

func TestMCAPToMatConversion(t *testing.T) {
	dir := t.TempDir()
	mcapPath := filepath.Join(dir, "input.mcap")
	writeTestMCAP(t, mcapPath)

	jobDir := t.TempDir()
	job := &FileJob{
		ID:       "job-1",
		Filename: filepath.Base(mcapPath),
		FilePath: mcapPath,
		FileDir:  jobDir,
	}

	results, err := (&PostProcessMCAPUploadJob{}).publishMCAPMessages(
		context.Background(),
		job,
		job.ID,
		map[string]messaging.SubscriberFunc{messaging.MAT: messaging.CreateRawMatFile},
	)
	if err != nil {
		t.Fatalf("publishMCAPMessages: %v", err)
	}

	result, ok := results[messaging.MAT]
	if !ok {
		t.Fatalf("MAT subscriber returned no result: %v", results)
	}
	matPath, ok := result.ResultData["file_path"].(string)
	if !ok {
		t.Fatalf("MAT subscriber returned no file path: %v", result.ResultData)
	}
	if want := filepath.Join(jobDir, job.ID+".mat"); matPath != want {
		t.Fatalf("MAT file path = %q, want %q", matPath, want)
	}
	if _, err := os.Stat(matPath); err != nil {
		t.Fatalf("MAT file %q: %v", matPath, err)
	}

	file, err := hdf5.OpenFile(matPath, hdf5.F_ACC_RDONLY)
	if err != nil {
		t.Fatalf("open MAT file: %v", err)
	}
	t.Cleanup(func() { file.Close() })

	data, err := file.OpenGroup("data")
	if err != nil {
		t.Fatalf("open /data: %v", err)
	}
	t.Cleanup(func() { data.Close() })
	if got := readTestMatlabClass(t, data); got != "struct" {
		t.Fatalf("/data MATLAB_class = %q, want struct", got)
	}

	topic, err := data.OpenGroup("Topic")
	if err != nil {
		t.Fatalf("open /data/Topic: %v", err)
	}
	t.Cleanup(func() { topic.Close() })
	if got := readTestMatlabClass(t, topic); got != "struct" {
		t.Fatalf("/data/Topic MATLAB_class = %q, want struct", got)
	}

	speedData := openMatTestDataset(t, topic, "speed", "Data")
	speedTime := openMatTestDataset(t, topic, "speed", "Timestamp")
	if got := readTestMatlabClass(t, speedData); got != "double" {
		t.Fatalf("speed Data MATLAB_class = %q, want double", got)
	}
	if got := readTestMatlabClass(t, speedTime); got != "double" {
		t.Fatalf("speed Timestamp MATLAB_class = %q, want double", got)
	}
	if got := readMatTestFloat64(t, speedData); !equalMatTestFloats(got, []float64{1.5, 1.5, 1.5}) {
		t.Fatalf("speed Data = %v, want [1.5 1.5 1.5]", got)
	}
	if got := readMatTestFloat64(t, speedTime); !equalMatTestFloats(got, []float64{0, 0.01, 0.02}) {
		t.Fatalf("speed Timestamp = %v, want [0 0.01 0.02]", got)
	}

	armedData := openMatTestDataset(t, topic, "armed", "Data")
	if got := readTestMatlabClass(t, armedData); got != "logical" {
		t.Fatalf("armed Data MATLAB_class = %q, want logical", got)
	}
	if got := readMatTestUint8(t, armedData); !slices.Equal(got, []uint8{1, 1, 1}) {
		t.Fatalf("armed Data = %v, want [1 1 1]", got)
	}

	if _, err := topic.OpenGroup("label"); err == nil {
		t.Fatal("string signal was written to the MAT file")
	}

	nested, err := topic.OpenGroup("nested")
	if err != nil {
		t.Fatalf("open /data/Topic/nested: %v", err)
	}
	t.Cleanup(func() { nested.Close() })

	tempData := openMatTestDataset(t, nested, "temp", "Data")
	tempTime := openMatTestDataset(t, nested, "temp", "Timestamp")
	if got := readTestMatlabClass(t, tempData); got != "double" {
		t.Fatalf("temp Data MATLAB_class = %q, want double", got)
	}
	if got := readMatTestFloat64(t, tempData); !equalMatTestFloats(got, []float64{20, 20, 20}) {
		t.Fatalf("temp Data = %v, want [20 20 20]", got)
	}
	if got := readMatTestFloat64(t, tempTime); !equalMatTestFloats(got, []float64{0, 0.01, 0.02}) {
		t.Fatalf("temp Timestamp = %v, want [0 0.01 0.02]", got)
	}
}

func writeTestMCAP(t *testing.T, path string) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create MCAP: %v", err)
	}
	defer file.Close()

	// The reader calls Info before Messages. A file without chunk indexes leaves
	// the stream at the end, so the iterator finds no messages.
	writer, err := mcap.NewWriter(file, &mcap.WriterOptions{Chunked: true, ChunkSize: 1024, IncludeCRC: true})
	if err != nil {
		t.Fatalf("create MCAP writer: %v", err)
	}
	if err := writer.WriteHeader(&mcap.Header{}); err != nil {
		t.Fatalf("write MCAP header: %v", err)
	}

	const schemaID uint16 = 1
	const channelID uint16 = 1
	if err := writer.WriteSchema(&mcap.Schema{
		ID:       schemaID,
		Name:     "test.Topic",
		Encoding: "jsonschema",
		Data:     []byte(`{"type":"object"}`),
	}); err != nil {
		t.Fatalf("write MCAP schema: %v", err)
	}
	if err := writer.WriteChannel(&mcap.Channel{
		ID:              channelID,
		SchemaID:        schemaID,
		Topic:           "test.Topic",
		MessageEncoding: "json",
		Metadata:        map[string]string{},
	}); err != nil {
		t.Fatalf("write MCAP channel: %v", err)
	}

	payload := []byte(`{"speed": 1.5, "nested": {"temp": 20}, "armed": true, "label": "idle"}`)
	for _, logTime := range []uint64{1_000_000_000, 1_010_000_000, 1_020_000_000} {
		if err := writer.WriteMessage(&mcap.Message{
			ChannelID:   channelID,
			LogTime:     logTime,
			PublishTime: logTime,
			Data:        payload,
		}); err != nil {
			t.Fatalf("write MCAP message: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close MCAP writer: %v", err)
	}
}

func openMatTestDataset(t *testing.T, group *hdf5.Group, signal, dataset string) *hdf5.Dataset {
	t.Helper()

	signalGroup, err := group.OpenGroup(signal)
	if err != nil {
		t.Fatalf("open group %s: %v", signal, err)
	}
	t.Cleanup(func() { signalGroup.Close() })

	value, err := signalGroup.OpenDataset(dataset)
	if err != nil {
		t.Fatalf("open dataset %s/%s: %v", signal, dataset, err)
	}
	t.Cleanup(func() { value.Close() })
	return value
}

type matTestAttributeOpener interface {
	OpenAttribute(name string) (*hdf5.Attribute, error)
}

func readTestMatlabClass(t *testing.T, object matTestAttributeOpener) string {
	t.Helper()

	attr, err := object.OpenAttribute("MATLAB_class")
	if err != nil {
		t.Fatalf("open MATLAB_class: %v", err)
	}
	defer attr.Close()

	dtype := &hdf5.Datatype{Identifier: attr.GetType()}
	defer dtype.Close()

	var raw [64]byte
	if err := attr.Read(&raw, dtype); err != nil {
		t.Fatalf("read MATLAB_class: %v", err)
	}
	return string(raw[:dtype.Size()])
}

func readMatTestFloat64(t *testing.T, dataset *hdf5.Dataset) []float64 {
	t.Helper()

	space := dataset.Space()
	defer space.Close()

	dims, _, err := space.SimpleExtentDims()
	if err != nil {
		t.Fatalf("dataset dims: %v", err)
	}
	values := make([]float64, dims[0])
	if err := dataset.Read(&values); err != nil {
		t.Fatalf("read float64 dataset: %v", err)
	}
	return values
}

func readMatTestUint8(t *testing.T, dataset *hdf5.Dataset) []uint8 {
	t.Helper()

	space := dataset.Space()
	defer space.Close()

	dims, _, err := space.SimpleExtentDims()
	if err != nil {
		t.Fatalf("dataset dims: %v", err)
	}
	values := make([]uint8, dims[0])
	if err := dataset.Read(&values); err != nil {
		t.Fatalf("read uint8 dataset: %v", err)
	}
	return values
}

func equalMatTestFloats(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			return false
		}
	}
	return true
}
