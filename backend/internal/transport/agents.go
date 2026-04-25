package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"arenea/backend/internal/domain"
)

func (h *HTTPHandler) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := h.agentSvc.Search(agentListQueryFromRequest(r))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var in domain.Agent
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		created, err := h.agentSvc.Create(in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("create", "agent", created.ID, r.Header.Get("X-Request-Id"), created.AgentKey)
		writeJSON(w, http.StatusCreated, created)
	default:
		methodNotAllowed(w)
	}
}

func agentListQueryFromRequest(r *http.Request) domain.AgentListQuery {
	q := r.URL.Query()
	return domain.AgentListQuery{
		Keyword:    q.Get("keyword"),
		Status:     q.Get("status"),
		Provider:   q.Get("provider"),
		CategoryID: q.Get("category_id"),
		Limit:      intQuery(q.Get("limit"), 24),
		Offset:     intQuery(q.Get("offset"), 0),
	}
}

func intQuery(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func (h *HTTPHandler) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	id := idFromPath(r.URL.Path, "/api/v1/agents/")
	if strings.HasSuffix(id, "/system-prompt/preview") {
		h.handleAgentPromptPreview(w, r, strings.TrimSuffix(id, "/system-prompt/preview"))
		return
	}
	if id == "" {
		writeErr(w, http.StatusBadRequest, errors.New("agent id is required"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		agent, err := h.agentSvc.Get(id)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, agent)
	case http.MethodPatch:
		var in domain.Agent
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		updated, err := h.agentSvc.Update(id, in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("update", "agent", updated.ID, r.Header.Get("X-Request-Id"), updated.AgentKey)
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := h.agentSvc.Delete(id); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("delete", "agent", id, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleAgentPromptPreview(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if id == "" {
		writeErr(w, http.StatusBadRequest, errors.New("agent id is required"))
		return
	}
	preview, err := h.agentSvc.PromptPreview(id, r.URL.Query().Get("mode"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"preview": preview})
}
