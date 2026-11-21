package background

// Benchmarks for the MCAP -> MAT conversion pipeline. S3, Mongo and the PNG plots are left out.
//
// Each stage adds one step on top of the previous one, so the differences show where the time goes:
//
//	BenchmarkMCAPRead     open the file, load schemas, iterate every record
//	BenchmarkMCAPDecode   ... + decode every message (GetDecodedMessage)
//	BenchmarkMCAPConvert  ... + publish to the MAT subscriber and write the .mat (the real conversion)
//
// Decoding and MAT writing run on separate goroutines in Convert, so (Convert - Decode) is only a
// rough estimate of the MAT cost. Use -cpuprofile for an exact breakdown.
//
// MCAP_BENCH is an .mcap file or a directory of them; every file becomes a sub-benchmark.
// MCAP_BENCH_KEEP_MAT is an optional directory to write the .mat into (and keep it) for diffing outputs.
//
// Usage (from cloud_webserver_v2/, with HDF5 from homebrew):
//
//	CGO_CFLAGS=-I/opt/homebrew/include CGO_LDFLAGS=-L/opt/homebrew/lib MCAP_BENCH=/path/to/mcaps \
//	    go test ./internal/background -run '^$' -bench MCAP -benchtime 3x -count 10 -timeout 0 | tee old.txt
//	benchstat old.txt new.txt

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/foxglove/mcap/go/mcap"
	"github.com/hytech-racing/cloud-webserver-v2/internal/messaging"
	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
)

func BenchmarkMCAPRead(b *testing.B) {
	forEachMCAP(b, func(b *testing.B, path string) {
		for i := 0; i < b.N; i++ {
			iterateMCAP(b, path, nil)
		}
	})
}

func BenchmarkMCAPDecode(b *testing.B) {
	forEachMCAP(b, func(b *testing.B, path string) {
		var decodeErrors int
		for i := 0; i < b.N; i++ {
			decodeErrors = 0
			iterateMCAP(b, path, func(mcapUtils *utils.McapUtils, schema *mcap.Schema, message *mcap.Message) {
				if _, err := mcapUtils.GetDecodedMessage(schema, message); err != nil {
					decodeErrors++
				}
			})
		}
		if decodeErrors > 0 {
			b.ReportMetric(float64(decodeErrors), "decode_errors/op")
		}
	})
}

func BenchmarkMCAPConvert(b *testing.B) {
	forEachMCAP(b, func(b *testing.B, path string) {
		outDir := os.Getenv("MCAP_BENCH_KEEP_MAT")
		if outDir == "" {
			outDir = b.TempDir()
		}

		job := &FileJob{
			ID:       "bench",
			Filename: filepath.Base(path),
			FilePath: path,
			FileDir:  outDir,
		}
		subscriberMapping := map[string]messaging.SubscriberFunc{
			messaging.MAT: messaging.CreateRawMatFile,
		}

		var matSize int64
		for i := 0; i < b.N; i++ {
			results, err := (&PostProcessMCAPUploadJob{}).publishMCAPMessages(context.Background(), job, job.ID, subscriberMapping)
			if err != nil {
				b.Fatal(err)
			}

			b.StopTimer()
			matPath, _ := results[messaging.MAT].ResultData["file_path"].(string)
			info, err := os.Stat(matPath)
			if err != nil {
				b.Fatalf("mat subscriber produced no file: %v", err)
			}
			matSize = info.Size()
			if os.Getenv("MCAP_BENCH_KEEP_MAT") == "" {
				os.Remove(matPath)
			}
			b.StartTimer()
		}
		b.ReportMetric(float64(matSize)/1e6, "mat_MB")
	})
}

// forEachMCAP runs bench as a sub-benchmark for every file in MCAP_BENCH and reports
// MB/s (from the input size) and msgs/s (from the MCAP summary statistics).
func forEachMCAP(b *testing.B, bench func(b *testing.B, path string)) {
	paths := benchMCAPFiles(b)

	prevLogOutput := log.Writer()
	log.SetOutput(io.Discard)
	b.Cleanup(func() { log.SetOutput(prevLogOutput) })

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			b.Fatal(err)
		}
		messageCount := mcapMessageCount(b, path)

		b.Run(filepath.Base(path), func(b *testing.B) {
			b.SetBytes(info.Size())
			b.ReportAllocs()
			bench(b, path)
			if messageCount > 0 {
				b.ReportMetric(float64(messageCount)*float64(b.N)/b.Elapsed().Seconds(), "msgs/s")
			}
		})
	}
}

func benchMCAPFiles(b *testing.B) []string {
	target := os.Getenv("MCAP_BENCH")
	if target == "" {
		b.Skip("set MCAP_BENCH to an .mcap file or a directory of them")
	}

	info, err := os.Stat(target)
	if err != nil {
		b.Fatalf("MCAP_BENCH: %v", err)
	}
	if !info.IsDir() {
		return []string{target}
	}

	paths, err := filepath.Glob(filepath.Join(target, "*.mcap"))
	if err != nil {
		b.Fatal(err)
	}
	if len(paths) == 0 {
		b.Fatalf("MCAP_BENCH: no .mcap files in %s", target)
	}
	sort.Strings(paths)
	return paths
}

// mcapMessageCount reads the message count from the MCAP summary section, or returns 0 if it has none.
func mcapMessageCount(b *testing.B, path string) uint64 {
	f, err := os.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	reader, err := mcap.NewReader(f)
	if err != nil {
		b.Fatal(err)
	}
	info, err := reader.Info()
	if err != nil || info.Statistics == nil {
		return 0
	}
	return info.Statistics.MessageCount
}

// iterateMCAP reads every message the same way publishMCAPMessages does and passes it to handle, if set.
func iterateMCAP(b *testing.B, path string, handle func(*utils.McapUtils, *mcap.Schema, *mcap.Message)) {
	f, err := os.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	mcapUtils := utils.NewMcapUtils()
	mcapReader, err := mcapUtils.NewReader(f)
	if err != nil {
		b.Fatal(err)
	}
	iterator, err := mcapReader.Reader.Messages()
	if err != nil {
		b.Fatal(err)
	}

	for {
		schema, _, message, err := iterator.NextInto(nil)
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			b.Fatal(err)
		}
		if schema == nil || handle == nil {
			continue
		}
		handle(mcapUtils, schema, message)
	}
}
