package transport

import (
	"encoding/json"
	"errors"
	"net/http"

	"arenea/backend/internal/domain"
)

func (h *HTTPHandler) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		agentID := r.URL.Query().Get("agent_id")
		teamID := r.URL.Query().Get("team_id")
		var (
			list []domain.Session
			err  error
		)
		switch {
		case teamID != "":
			list, err = h.sessionSvc.ListTeam(teamID)
		default:
			list, err = h.sessionSvc.List(agentID)
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse[domain.Session]{Items: list})
	case http.MethodPost:
		var in domain.Session
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		created, err := h.sessionSvc.Create(in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("create", "session", created.ID, r.Header.Get("X-Request-Id"), created.Title)
		writeJSON(w, http.StatusCreated, created)
	case http.MethodDelete:
		agentID := r.URL.Query().Get("agent_id")
		if agentID == "" {
			writeErr(w, http.StatusBadRequest, errors.New("agent_id is required"))
			return
		}
		if err := h.sessionSvc.DeleteByAgent(agentID); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("delete", "sessions", agentID, r.Header.Get("X-Request-Id"), "clear history")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	id := idFromPath(r.URL.Path, "/api/v1/sessions/")
	if id == "" {
		writeErr(w, http.StatusBadRequest, errors.New("session id is required"))
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := h.sessionSvc.Delete(id); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("delete", "session", id, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}
