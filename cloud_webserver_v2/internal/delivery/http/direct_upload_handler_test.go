package http

import (
	"net/http"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
)

func TestSanitizeUploadFileName(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		accept bool
	}{
		{name: "plain", input: "run.mcap", accept: true},
		{name: "uppercase extension", input: "run.MCAP", accept: true},
		{name: "surrounding space", input: "  run.mcap  ", accept: true},
		{name: "empty", input: "", accept: false},
		{name: "current directory", input: ".", accept: false},
		{name: "parent directory", input: "..", accept: false},
		{name: "forward slash", input: "dir/run.mcap", accept: false},
		{name: "backslash", input: `dir\run.mcap`, accept: false},
		{name: "control character", input: "run\n.mcap", accept: false},
		{name: "wrong extension", input: "run.txt", accept: false},
		{name: "too long", input: strings.Repeat("a", 251) + ".mcap", accept: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, handlerErr := sanitizeUploadFileName(test.input)
			if (handlerErr == nil) != test.accept {
				t.Fatalf("sanitizeUploadFileName(%q) error = %v, want accepted %v", test.input, handlerErr, test.accept)
			}
		})
	}
}

func TestFinalizeParts(t *testing.T) {
	t.Run("no multipart fields", func(t *testing.T) {
		parts, handlerErr := finalizeParts(finalizeUploadRequest{Key: "key"})
		if handlerErr != nil || parts != nil {
			t.Fatalf("finalizeParts() = %v, %v, want nil, nil", parts, handlerErr)
		}
	})

	rejected := []struct {
		name    string
		request finalizeUploadRequest
	}{
		{
			name:    "parts without upload id",
			request: finalizeUploadRequest{Parts: []completedPartRequest{{PartNumber: 1, ETag: `"a"`}}},
		},
		{
			name:    "upload id without parts",
			request: finalizeUploadRequest{UploadID: "upload-id"},
		},
		{
			name:    "zero part number",
			request: finalizeUploadRequest{UploadID: "upload-id", Parts: []completedPartRequest{{PartNumber: 0, ETag: `"a"`}}},
		},
	}

	for _, test := range rejected {
		t.Run(test.name, func(t *testing.T) {
			if _, handlerErr := finalizeParts(test.request); handlerErr == nil || handlerErr.StatusCode != http.StatusBadRequest {
				t.Fatalf("finalizeParts(%v) error = %v, want status %d", test.request, handlerErr, http.StatusBadRequest)
			}
		})
	}

	t.Run("sorts parts and preserves raw etags", func(t *testing.T) {
		parts, handlerErr := finalizeParts(finalizeUploadRequest{
			UploadID: "upload-id",
			Parts: []completedPartRequest{
				{PartNumber: 3, ETag: `"c"`},
				{PartNumber: 1, ETag: `"a"`},
				{PartNumber: 2, ETag: `"b"`},
			},
		})
		if handlerErr != nil {
			t.Fatalf("finalizeParts() error: %v", handlerErr)
		}
		want := []s3.MultipartPart{
			{PartNumber: 1, ETag: `"a"`},
			{PartNumber: 2, ETag: `"b"`},
			{PartNumber: 3, ETag: `"c"`},
		}
		if !slices.Equal(parts, want) {
			t.Fatalf("finalizeParts() = %v, want %v", parts, want)
		}
	})
}

func TestValidatePartNumbers(t *testing.T) {
	numbers, handlerErr := validatePartNumbers([]int32{3, 1, 2}, 3)
	if handlerErr != nil || !slices.Equal(numbers, []int32{1, 2, 3}) {
		t.Fatalf("validatePartNumbers sorted = %v, %v, want [1 2 3]", numbers, handlerErr)
	}

	tests := []struct {
		name    string
		numbers []int32
		limit   int
	}{
		{name: "empty", numbers: nil, limit: 10},
		{name: "over limit", numbers: []int32{1, 2, 3}, limit: 2},
		{name: "zero", numbers: []int32{0}, limit: 10},
		{name: "above max", numbers: []int32{maxPartNumber + 1}, limit: 10},
		{name: "duplicate", numbers: []int32{1, 1}, limit: 10},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, handlerErr := validatePartNumbers(test.numbers, test.limit); handlerErr == nil {
				t.Fatalf("validatePartNumbers(%v, %d) accepted", test.numbers, test.limit)
			}
		})
	}
}

func TestRegistryRejectsForeignAndExpiredUploads(t *testing.T) {
	registry := newDirectUploadRegistry()
	registry.add(&directUploadSession{Key: "live", Owner: "alice", ExpiresAt: time.Now().Add(time.Hour)})
	registry.add(&directUploadSession{Key: "expired", Owner: "alice", ExpiresAt: time.Now().Add(-time.Hour)})

	if _, err := registry.lookup("live", "bob"); err == nil {
		t.Fatal("lookup accepted a session owned by another user")
	}
	if _, err := registry.lookup("expired", "alice"); err == nil {
		t.Fatal("lookup accepted an expired session")
	}
	if _, err := registry.lookup("live", "alice"); err != nil {
		t.Fatalf("lookup rejected a live session: %v", err)
	}
}

func TestClaimFinalizeReturnsSameJobAfterCompletion(t *testing.T) {
	registry := newDirectUploadRegistry()
	registry.add(&directUploadSession{Key: "key", Owner: "alice", ExpiresAt: time.Now().Add(time.Hour)})

	session, jobID, err := registry.claimFinalize("key", "alice")
	if err != nil || jobID != "" || session.Key != "key" {
		t.Fatalf("first claimFinalize = %v, %q, %v", session, jobID, err)
	}
	registry.finishFinalize("key", "job-1")

	_, jobID, err = registry.claimFinalize("key", "alice")
	if err != nil || jobID != "job-1" {
		t.Fatalf("retried claimFinalize job = %q, %v, want job-1", jobID, err)
	}
}

func TestClaimFinalizeRejectsConcurrentFinalize(t *testing.T) {
	registry := newDirectUploadRegistry()
	registry.add(&directUploadSession{Key: "key", Owner: "alice", ExpiresAt: time.Now().Add(time.Hour)})

	if _, _, err := registry.claimFinalize("key", "alice"); err != nil {
		t.Fatalf("first claimFinalize: %v", err)
	}
	if _, _, err := registry.claimFinalize("key", "alice"); err != errUploadFinalizing {
		t.Fatalf("second claimFinalize error = %v, want %v", err, errUploadFinalizing)
	}
}

func TestTakeExpiredOnlyRemovesExpiredSessions(t *testing.T) {
	registry := newDirectUploadRegistry()
	registry.add(&directUploadSession{Key: "live", Owner: "alice", ExpiresAt: time.Now().Add(time.Hour)})
	registry.add(&directUploadSession{Key: "expired", Owner: "alice", ExpiresAt: time.Now().Add(-time.Hour)})

	expired := registry.takeExpired(time.Now())
	if len(expired) != 1 || expired[0].Key != "expired" {
		t.Fatalf("takeExpired = %v, want only the expired session", expired)
	}
	if _, err := registry.lookup("live", "alice"); err != nil {
		t.Fatalf("takeExpired removed a live session: %v", err)
	}
}

func TestRecoverDirectUploadSession(t *testing.T) {
	prefix := path.Join(directUploadPrefix, "alice")
	validKey := prefix + "/" + strings.Repeat("a", directUploadIDLength) + "/run.mcap"

	session, ok := recoverDirectUploadSession(validKey, "alice")
	if !ok || session.FileName != "run.mcap" || session.Key != validKey {
		t.Fatalf("recoverDirectUploadSession(valid) = %v, %v", session, ok)
	}
	if _, ok := recoverDirectUploadSession(validKey, "bob"); ok {
		t.Fatal("recovered a key in another user's namespace")
	}
	if _, ok := recoverDirectUploadSession(prefix+"/short/run.mcap", "alice"); ok {
		t.Fatal("recovered a key with a malformed id")
	}
	if _, ok := recoverDirectUploadSession(prefix+"/"+strings.Repeat("a", directUploadIDLength)+"/run.txt", "alice"); ok {
		t.Fatal("recovered a key with a rejected file name")
	}
}
