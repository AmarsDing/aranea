package transport

import (
	"encoding/json"
	"net/http"
	"strings"
)

type listResponse[T any] struct {
	Items []T `json:"items"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorResponse{Error: err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
}

func idFromPath(path, prefix string) string {
	return strings.Trim(strings.TrimPrefix(path, prefix), "/")
}
