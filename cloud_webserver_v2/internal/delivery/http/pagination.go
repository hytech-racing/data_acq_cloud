package http

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const pageCursorVersion = "v1"

func parsePageRequest(q url.Values) (page models.PageRequest, paged bool, err error) {
	limitPresent := q.Has("limit")
	cursorPresent := q.Has("cursor")
	if !limitPresent && !cursorPresent {
		return models.PageRequest{}, false, nil
	}
	if !limitPresent {
		return models.PageRequest{}, false, errors.New("invalid cursor: limit is required when a cursor is set")
	}

	rawLimit := q.Get("limit")
	limit, err := strconv.Atoi(rawLimit)
	if err != nil {
		return models.PageRequest{}, false, fmt.Errorf("invalid limit: %q is not a number", rawLimit)
	}

	var after *models.PageCursor
	if cursorPresent {
		cursor, err := decodePageCursor(q.Get("cursor"))
		if err != nil {
			return models.PageRequest{}, false, fmt.Errorf("invalid cursor: %w", err)
		}
		after = &cursor
	}

	page, err = models.NewPageRequest(limit, after)
	if err != nil {
		return models.PageRequest{}, false, err
	}
	return page, true, nil
}

func encodePageCursor(cursor models.PageCursor) string {
	raw := fmt.Sprintf("%s|%s|%s",
		pageCursorVersion,
		cursor.Date.UTC().Format(time.RFC3339Nano),
		cursor.ID.Hex(),
	)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodePageCursor(encoded string) (models.PageCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return models.PageCursor{}, errors.New("malformed token")
	}

	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return models.PageCursor{}, errors.New("malformed token")
	}
	if parts[0] != pageCursorVersion {
		return models.PageCursor{}, fmt.Errorf("unsupported cursor version %q", parts[0])
	}

	date, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil {
		return models.PageCursor{}, errors.New("invalid timestamp")
	}

	id, err := primitive.ObjectIDFromHex(parts[2])
	if err != nil {
		return models.PageCursor{}, errors.New("invalid id")
	}

	return models.PageCursor{Date: date, ID: id}, nil
}
