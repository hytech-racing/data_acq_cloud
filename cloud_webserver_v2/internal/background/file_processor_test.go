package background

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestHashStagedFile(t *testing.T) {
	data := []byte("known bytes for hashStagedFile")
	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])

	path := filepath.Join(t.TempDir(), "staged.mcap")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("os.Open() error: %v", err)
	}
	defer file.Close()

	got, err := hashStagedFile(file, int64(len(data)), int64(len(data)))
	if err != nil {
		t.Fatalf("hashStagedFile() error: %v", err)
	}
	if got != want {
		t.Errorf("hashStagedFile() = %q, want %q", got, want)
	}

	if _, err := hashStagedFile(file, int64(len(data)), int64(len(data))-1); err == nil {
		t.Error("hashStagedFile() with short count returned nil error")
	}
	if _, err := hashStagedFile(file, int64(len(data)), int64(len(data))+1); err == nil {
		t.Error("hashStagedFile() with long count returned nil error")
	}
}
