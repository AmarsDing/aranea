package transport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/domain"
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
	status, message := publicError(status, err)
	writeJSON(w, status, errorResponse{Error: message})
}

// decodeBody 将 JSON 请求体解码到 target；载荷无效时写入 400 错误响应。
// 返回布尔值表示成功，调用方可在失败时立即 return，无需再次检查 error。
func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
}

func idFromPath(path, prefix string) string {
	return strings.Trim(strings.TrimPrefix(path, prefix), "/")
}

func publicError(fallbackStatus int, err error) (int, string) {
	if err == nil {
		return fallbackStatus, http.StatusText(fallbackStatus)
	}
	status := statusForError(fallbackStatus, err)
	switch {
	case status >= http.StatusInternalServerError:
		return status, "internal server error"
	case status == http.StatusNotFound:
		return status, "resource not found"
	case status == http.StatusUnauthorized:
		return status, "unauthorized"
	case status == http.StatusConflict:
		// L1 溢出 / 修订冲突 / 任务不可写等错误带有可操作的明细（字段、修订、任务），
		// 前端可在 toast 中展示。
		switch {
		case errors.Is(err, domain.ErrL1Overflow),
			errors.Is(err, domain.ErrRevisionConflict),
			errors.Is(err, domain.ErrTaskNotWritable):
			return status, err.Error()
		default:
			return status, "conflict"
		}
	case status == 499:
		return status, "request cancelled"
	default:
		return status, err.Error()
	}
}

func statusForError(fallbackStatus int, err error) int {
	switch {
	case errors.Is(err, domain.ErrValidation),
		errors.Is(err, domain.ErrInvalidFieldPath),
		errors.Is(err, domain.ErrInvalidFieldValue):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrFieldTooLarge):
		return http.StatusUnprocessableEntity
	case errors.Is(err, domain.ErrConflict),
		errors.Is(err, domain.ErrL1Overflow),
		errors.Is(err, domain.ErrRevisionConflict),
		errors.Is(err, domain.ErrTaskNotWritable):
		return http.StatusConflict
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, domain.ErrInternal):
		return http.StatusInternalServerError
	case errors.Is(err, context.Canceled):
		return 499
	}
	if fallbackStatus <= 0 {
		return http.StatusInternalServerError
	}
	return fallbackStatus
}
