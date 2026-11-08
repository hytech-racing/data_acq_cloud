package http

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestDirectUploadRoutesAreRegistered(t *testing.T) {
	router := chi.NewRouter()
	NewUploadHandler(router, nil, nil, nil, nil)

	registered := make(map[string]bool)
	err := chi.Walk(router, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		if method == http.MethodPost {
			registered[route] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("chi.Walk() returned unexpected error: %v", err)
	}

	for _, route := range []string{
		"/uploads/presign",
		"/uploads/initiate",
		"/uploads/parts",
		"/uploads/complete",
		"/uploads/abort",
		"/uploads/finalize",
	} {
		if !registered[route] {
			t.Errorf("route %s is not registered", route)
		}
	}
}

func TestSanitizeUploadFileName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "mcap file", input: "run_2026-11-08.mcap", want: "run_2026-11-08.mcap"},
		{name: "case insensitive extension", input: "RUN.MCAP", want: "RUN.MCAP"},
		{name: "trims whitespace", input: "  run.mcap  ", want: "run.mcap"},
		{name: "missing name", input: "", wantErr: true},
		{name: "directory traversal", input: "../run.mcap", wantErr: true},
		{name: "nested path", input: "runs/run.mcap", wantErr: true},
		{name: "windows separator", input: `runs\run.mcap`, wantErr: true},
		{name: "dot", input: ".", wantErr: true},
		{name: "dot dot", input: "..", wantErr: true},
		{name: "wrong extension", input: "run.h5", wantErr: true},
		{name: "no extension", input: "run", wantErr: true},
		{name: "control character", input: "run\x00.mcap", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, handlerErr := sanitizeUploadFileName(test.input)
			if test.wantErr {
				if handlerErr == nil {
					t.Fatalf("sanitizeUploadFileName(%q) = %q, want an error", test.input, got)
				}
				return
			}
			if handlerErr != nil {
				t.Fatalf("sanitizeUploadFileName(%q) returned unexpected error: %v", test.input, handlerErr.Message)
			}
			if got != test.want {
				t.Errorf("sanitizeUploadFileName(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestNormalizeETag(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: `"abc123"`, want: `"abc123"`},
		{input: "abc123", want: `"abc123"`},
		{input: ` "abc123" `, want: `"abc123"`},
		{input: "", wantErr: true},
		{input: `"abc"123"`, wantErr: true},
	}

	for _, test := range tests {
		got, err := normalizeETag(test.input)
		if test.wantErr {
			if err == nil {
				t.Errorf("normalizeETag(%q) = %q, want an error", test.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeETag(%q) returned unexpected error: %v", test.input, err)
			continue
		}
		if got != test.want {
			t.Errorf("normalizeETag(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestDirectUploadRegistryFinalizeIsIdempotent(t *testing.T) {
	registry := newDirectUploadRegistry()
	session := &directUploadSession{
		Key:       "direct-uploads/user/id/run.mcap",
		Owner:     "user",
		FileName:  "run.mcap",
		Size:      1024,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	registry.add(session)

	claimed, jobID, err := registry.claimFinalize(session.Key, "user")
	if err != nil {
		t.Fatalf("claimFinalize() returned unexpected error: %v", err)
	}
	if jobID != "" {
		t.Fatalf("claimFinalize() returned job id %q before one was set", jobID)
	}
	if claimed.FileName != session.FileName {
		t.Errorf("claimFinalize() returned file name %q, want %q", claimed.FileName, session.FileName)
	}

	if _, _, err := registry.claimFinalize(session.Key, "user"); !errors.Is(err, errUploadFinalizing) {
		t.Errorf("second claimFinalize() error = %v, want %v", err, errUploadFinalizing)
	}

	registry.finishFinalize(session.Key, "job_1")

	_, jobID, err = registry.claimFinalize(session.Key, "user")
	if err != nil {
		t.Fatalf("claimFinalize() after completion returned unexpected error: %v", err)
	}
	if jobID != "job_1" {
		t.Errorf("claimFinalize() after completion returned job id %q, want %q", jobID, "job_1")
	}
}

func TestDirectUploadRegistryRejectsOtherOwners(t *testing.T) {
	registry := newDirectUploadRegistry()
	registry.add(&directUploadSession{
		Key:       "direct-uploads/user/id/run.mcap",
		Owner:     "user",
		ExpiresAt: time.Now().Add(time.Hour),
	})

	if _, err := registry.lookup("direct-uploads/user/id/run.mcap", "someone-else"); !errors.Is(err, errUploadNotFound) {
		t.Errorf("lookup() error = %v, want %v", err, errUploadNotFound)
	}
	if _, err := registry.take("direct-uploads/user/id/run.mcap", "someone-else"); !errors.Is(err, errUploadNotFound) {
		t.Errorf("take() error = %v, want %v", err, errUploadNotFound)
	}
}

func TestDirectUploadRegistryExpiresSessions(t *testing.T) {
	registry := newDirectUploadRegistry()
	registry.add(&directUploadSession{
		Key:       "direct-uploads/user/id/run.mcap",
		Owner:     "user",
		ExpiresAt: time.Now().Add(-time.Minute),
	})

	if _, err := registry.lookup("direct-uploads/user/id/run.mcap", "user"); !errors.Is(err, errUploadNotFound) {
		t.Errorf("lookup() error = %v, want %v", err, errUploadNotFound)
	}

	expired := registry.takeExpired(time.Now())
	if len(expired) != 1 {
		t.Errorf("takeExpired() returned %d sessions, want 1", len(expired))
	}
}
