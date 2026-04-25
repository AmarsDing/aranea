package repository

import (
	"encoding/json"
	"strings"
	"time"
)

// scanner abstracts *sql.Row and *sql.Rows so helper scan functions can read
// from either single-row or multi-row queries.
type scanner interface {
	Scan(dest ...any) error
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// optionalColumn currently passes the column name through unchanged. It is kept
// as a single point of indirection in case migrations introduce columns lazily
// and we need to swap in NULL placeholders.
func optionalColumn(_ string, column string) string {
	return column
}

// previewText returns the first `limit` runes of value with an ellipsis suffix.
// When limit is non-positive or value already fits, value is returned trimmed.
func previewText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}

// normalizeJSONList ensures the persisted value is a valid JSON array. A bare
// string is wrapped into a single-element array; empty input becomes "[]".
func normalizeJSONList(value string) string {
	if strings.TrimSpace(value) == "" {
		return "[]"
	}
	if json.Valid([]byte(value)) {
		return value
	}
	encoded, err := json.Marshal([]string{value})
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

// sanitizePromptFileID converts a free-form prompt file name into a stable id
// suitable for use in primary keys.
func sanitizePromptFileID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, value)
	return strings.Trim(value, "_")
}
