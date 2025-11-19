package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// Reads the contents of an os.File and returns its SHA-256 hash as a string
func CreateFileHash(file *os.File) (string, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
