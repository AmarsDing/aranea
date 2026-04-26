package repository

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"time"
)

// idCounter monotonically increments to disambiguate IDs minted in the
// same nanosecond — crucial for tight insertion loops (tests, batched
// telemetry) where UnixNano() alone collides.
var idCounter atomic.Uint64

// uniqueID composes a sortable, collision-resistant ID. The ns prefix
// keeps natural chronological ordering while the counter suffix
// guarantees uniqueness when called repeatedly within the same ns.
func uniqueID(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UTC().UnixNano(), idCounter.Add(1))
}

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

// decodeJSONFloatMap parses a `{"key":number}` JSON object into a Go map.
// Empty / invalid input yields nil. Used by the L4 agent evolution
// repository for tool / provider / model preference columns.
func decodeJSONFloatMap(raw string) map[string]float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	out := map[string]float64{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// encodeJSONFloatMap is the inverse of decodeJSONFloatMap. Empty maps
// serialise to "{}" so the column never holds an SQL-illegal empty string.
func encodeJSONFloatMap(in map[string]float64) string {
	if len(in) == 0 {
		return "{}"
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// EncodeFloat32Blob serialises a float32 vector as little-endian bytes so
// it round-trips through SQLite BLOB columns.
func EncodeFloat32Blob(vec []float32) []byte {
	if len(vec) == 0 {
		return nil
	}
	out := make([]byte, 4*len(vec))
	for i, f := range vec {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(f))
	}
	return out
}

// decodeFloat32Blob is the inverse of EncodeFloat32Blob. Returns an error
// when the byte length isn't a multiple of 4.
func decodeFloat32Blob(blob []byte) ([]float32, error) {
	if len(blob)%4 != 0 {
		return nil, errors.New("invalid float32 blob length")
	}
	out := make([]float32, len(blob)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return out, nil
}

// vectorNorm returns the L2 norm of a float32 vector. Used both for the
// query side of cosine similarity and to populate the embedding_norm
// column.
func vectorNorm(vec []float32) float64 {
	if len(vec) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum)
}

// dotProduct is the unrolled-friendly inner product used inside the
// vector recall path.
func dotProduct(a []float32, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
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
