package utils

import (
	"fmt"
	"io"

	"github.com/foxglove/mcap/go/mcap"
)

type mcapUtils struct{}

func NewMcapUtils() *mcapUtils {
	return &mcapUtils{}
}

func (m *mcapUtils) NewReader(r io.Reader) (*mcap.Reader, error) {
	reader, err := mcap.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to build reader: %w", err)
	}
	return reader, nil
}
