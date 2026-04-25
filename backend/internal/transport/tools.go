package transport

import (
	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/domain"
)

type toolListResponse struct {
	Items    []domain.Tool      `json:"items"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Total    int                `json:"total"`
	Summary  domain.ToolSummary `json:"summary"`
}

func (h *HTTPHandler) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	page, pageSize, offset := pageParams(r)
	result, err := h.toolSvc.Search(domain.ToolListQuery{
		Search:    r.URL.Query().Get("search"),
		Category:  r.URL.Query().Get("category"),
		Source:    r.URL.Query().Get("source"),
		RiskLevel: r.URL.Query().Get("risk_level"),
		Enabled:   r.URL.Query().Get("enabled"),
		Limit:     pageSize,
		Offset:    offset,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, toolListResponse{Items: result.Items, Page: page, PageSize: pageSize, Total: result.Total, Summary: result.Summary})
}

func (h *HTTPHandler) handleToolByID(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(idFromPath(r.URL.Path, "/api/v1/tools/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, errors.New("tool id is required"))
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		tool, err := h.toolSvc.Get(id)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, tool)
		return
	}
	if len(parts) == 2 && parts[1] == "enabled" {
		h.handleToolEnabled(w, r, id)
		return
	}
	methodNotAllowed(w)
}

func (h *HTTPHandler) handleToolEnabled(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w)
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	updated, err := h.toolSvc.ToggleEnabled(id, in.Enabled)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("update", "tools", updated.ID, r.Header.Get("X-Request-Id"), "enabled")
	writeJSON(w, http.StatusOK, updated)
}

func (h *HTTPHandler) handleToolRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	page, pageSize, offset := pageParams(r)
	result, err := h.toolSvc.SearchRuns(domain.ToolRunQuery{
		ToolKey:   r.URL.Query().Get("tool_key"),
		AgentID:   r.URL.Query().Get("agent_id"),
		SessionID: r.URL.Query().Get("session_id"),
		Status:    r.URL.Query().Get("status"),
		From:      r.URL.Query().Get("from"),
		To:        r.URL.Query().Get("to"),
		Limit:     pageSize,
		Offset:    offset,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, paginatedResponse[domain.ToolInvocation]{Items: result.Items, Page: page, PageSize: pageSize, Total: result.Total})
}
