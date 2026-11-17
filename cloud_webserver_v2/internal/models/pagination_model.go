package models

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const MaxPageSize = 200

// PageCursor is the sort tuple of the last row on the previous page.
type PageCursor struct {
	Date time.Time
	ID   primitive.ObjectID
}

type PageRequest struct {
	limit int
	after *PageCursor
}

func NewPageRequest(limit int, after *PageCursor) (PageRequest, error) {
	if limit < 1 || limit > MaxPageSize {
		return PageRequest{}, fmt.Errorf("invalid limit: must be between 1 and %d", MaxPageSize)
	}
	if after != nil && (after.Date.IsZero() || after.ID.IsZero()) {
		return PageRequest{}, fmt.Errorf("invalid cursor: date and id must be set")
	}
	return PageRequest{limit: limit, after: after}, nil
}

func (p PageRequest) Limit() int {
	return p.limit
}

func (p PageRequest) After() *PageCursor {
	return p.after
}

func (p PageRequest) IsZero() bool {
	return p.limit == 0
}

type VehicleRunPage struct {
	Runs []VehicleRunModel
	Next *PageCursor
}
