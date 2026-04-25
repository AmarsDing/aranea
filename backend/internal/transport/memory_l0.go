package transport

import (
	"errors"
	"net/http"

	"arenea/backend/internal/domain"
)

// handleL0Snapshots serves GET /api/v1/sessions/{id}/l0/snapshots. The list
// powers the "Context" tab in the chat UI and the agent-evolution dashboard.
// It intentionally returns the snapshot rows verbatim — UI / analytics decode
// `segments_json` / `metadata_json` themselves.
func (h *HTTPHandler) handleL0Snapshots(w http.ResponseWriter, r *http.Request, sessionID string) {
	if sessionID == "" {
		writeErr(w, http.StatusBadRequest, errors.New("session id is required"))
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 20)
	svc := h.chatSvc.MemoryL0()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L0 service is not configured"))
		return
	}
	snapshots, err := svc.ListSnapshots(r.Context(), sessionID, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": snapshots,
		"total": len(snapshots),
	})
}

// l0PreviewRequest is the minimal payload that lets the prompt-debugger
// reproduce an L0 assembly without persisting a snapshot. Reserved fields
// mirror the runtime so debugger output matches what the model would see.
type l0PreviewRequest struct {
	SessionID         string `json:"session_id"`
	AgentID           string `json:"agent_id"`
	TeamID            string `json:"team_id"`
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	ContextWindow     int    `json:"context_window"`
	ReservedForOutput int    `json:"reserved_for_output"`
	UserMessage       string `json:"user_message"`
}

// handleL0Preview serves POST /api/v1/l0/preview. Bodies are decoded into the
// minimal request above; the response carries redacted segments (preview
// only) so the UI can render the assembled prompt without leaking history
// content beyond what `messages` already exposes.
func (h *HTTPHandler) handleL0Preview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in l0PreviewRequest
	if !decodeBody(w, r, &in) {
		return
	}
	if in.SessionID == "" {
		writeErr(w, http.StatusBadRequest, errors.New("session_id is required"))
		return
	}
	svc := h.chatSvc.MemoryL0()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L0 service is not configured"))
		return
	}
	req := domain.L0AssemblyRequest{
		SessionID:         in.SessionID,
		AgentID:           in.AgentID,
		TeamID:            in.TeamID,
		Provider:          in.Provider,
		Model:             in.Model,
		ContextWindow:     in.ContextWindow,
		ReservedForOutput: in.ReservedForOutput,
		UserMessage:       in.UserMessage,
	}
	result, err := svc.Preview(r.Context(), req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleL0SnapshotByID serves GET /api/v1/l0/snapshots/{id}. It is the
// "deep-link" variant of the snapshot list so the chat UI can jump straight
// to one assembly's full segment table.
func (h *HTTPHandler) handleL0SnapshotByID(w http.ResponseWriter, r *http.Request) {
	id := idFromPath(r.URL.Path, "/api/v1/l0/snapshots/")
	if id == "" {
		writeErr(w, http.StatusBadRequest, errors.New("snapshot id is required"))
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	svc := h.chatSvc.MemoryL0()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L0 service is not configured"))
		return
	}
	snap, err := svc.GetSnapshot(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}
