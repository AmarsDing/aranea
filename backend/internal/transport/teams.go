package transport

import (
	"net/http"

	"arenea/backend/internal/domain"
)

func (h *HTTPHandler) handleTeams(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.teamSvc.List()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse[domain.Team]{Items: items})
	default:
		methodNotAllowed(w)
	}
}
