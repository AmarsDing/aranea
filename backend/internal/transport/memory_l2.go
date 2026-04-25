// transport/memory_l2.go exposes the L2 episodic-memory HTTP surface
// described in `aranea/docs/14 memory-L2-episodic.md` §6.2 – §6.6. The
// router lives in sessions.go (handleSessionByID) which forwards `/l2/`
// suffixes here. Admin endpoints (consolidation / retention) are wired
// in handler.go via registerMemoryL2AdminRoutes.
package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/service"
)

// registerMemoryL2AdminRoutes installs the admin-scoped retention and
// consolidation endpoints. They live under /api/v1/admin/memory/l2/.
func (h *HTTPHandler) registerMemoryL2AdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/admin/memory/l2/retention/run", h.handleL2RetentionRun)
}

// handleL2Routes dispatches /api/v1/sessions/{sid}/l2/<resource>... requests.
// Resources:
//   - events                         (GET)
//   - events/{ref_kind}/{ref_id}     (GET, Phase 2)
//   - episodes                       (GET, POST)
//   - episodes/{id}                  (GET, PATCH, DELETE)
//   - episodes/{id}/reindex          (POST)
//   - episodes/{id}/consolidate      (POST, Phase 3 stub)
//   - marks                          (GET, POST)
//   - marks/{id}                     (DELETE)
//   - recall                         (POST)
func (h *HTTPHandler) handleL2Routes(w http.ResponseWriter, r *http.Request, sessionID, suffix string) {
	svc := h.l2Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L2 service is not configured"))
		return
	}
	parts := strings.Split(suffix, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusNotFound, errors.New("unknown l2 path"))
		return
	}
	switch parts[0] {
	case "events":
		h.handleL2Events(w, r, svc, sessionID, parts[1:])
	case "episodes":
		h.handleL2Episodes(w, r, svc, sessionID, parts[1:])
	case "marks":
		h.handleL2Marks(w, r, svc, sessionID, parts[1:])
	case "recall":
		h.handleL2Recall(w, r, svc, sessionID)
	default:
		writeErr(w, http.StatusNotFound, errors.New("unknown l2 resource"))
	}
}

// --- Events -----------------------------------------------------------------

func (h *HTTPHandler) handleL2Events(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID string, parts []string) {
	if len(parts) > 0 && parts[0] != "" {
		// `events/{ref_kind}/{ref_id}` would land here; not implemented in
		// Phase 1 because clients can drill back through ref_table / ref_id
		// using existing per-table endpoints.
		writeErr(w, http.StatusNotImplemented, errors.New("event detail endpoint not implemented in phase 1"))
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	q := domain.MemoryL2EventQuery{
		SessionID:    sessionID,
		TurnID:       r.URL.Query().Get("turn_id"),
		SpanID:       r.URL.Query().Get("span_id"),
		Kinds:        splitCSV(r.URL.Query().Get("kinds")),
		ActorIDs:     splitCSV(r.URL.Query().Get("actor_id")),
		StatusIn:     splitCSV(r.URL.Query().Get("status")),
		StartTimeUTC: r.URL.Query().Get("start_time"),
		EndTimeUTC:   r.URL.Query().Get("end_time"),
		Keyword:      r.URL.Query().Get("keyword"),
		Limit:        parsePositiveInt(r.URL.Query().Get("limit"), 100),
		Offset:       parsePositiveInt(r.URL.Query().Get("offset"), 0),
	}
	result, err := svc.ListEvents(r.Context(), q)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- Episodes ---------------------------------------------------------------

func (h *HTTPHandler) handleL2Episodes(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID string, parts []string) {
	switch len(parts) {
	case 0:
		h.handleL2EpisodeCollection(w, r, svc, sessionID)
	case 1:
		h.handleL2EpisodeItem(w, r, svc, sessionID, parts[0])
	case 2:
		h.handleL2EpisodeAction(w, r, svc, sessionID, parts[0], parts[1])
	default:
		writeErr(w, http.StatusNotFound, errors.New("unknown episode path"))
	}
}

func (h *HTTPHandler) handleL2EpisodeCollection(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID string) {
	switch r.Method {
	case http.MethodGet:
		kind := r.URL.Query().Get("kind")
		limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
		offset := parsePositiveInt(r.URL.Query().Get("offset"), 0)
		result, err := svc.ListEpisodes(r.Context(), sessionID, kind, limit, offset)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodPost:
		var in service.CreateEpisodeInput
		if !decodeBody(w, r, &in) {
			return
		}
		in.SessionID = sessionID
		ep, err := svc.CreateMilestoneEpisode(r.Context(), in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l2.create_episode", "memory_episodes", ep.ID, r.Header.Get("X-Request-Id"), ep.Title)
		writeJSON(w, http.StatusCreated, ep)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleL2EpisodeItem(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID, episodeID string) {
	if episodeID == "" {
		writeErr(w, http.StatusBadRequest, errors.New("episode id is required"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		detail, err := svc.GetEpisode(r.Context(), episodeID)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		if detail.Episode.SessionID != sessionID {
			writeErr(w, http.StatusNotFound, errors.New("episode not found in session"))
			return
		}
		writeJSON(w, http.StatusOK, detail)
	case http.MethodPatch:
		current, err := svc.GetEpisode(r.Context(), episodeID)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		if current.Episode.SessionID != sessionID {
			writeErr(w, http.StatusNotFound, errors.New("episode not found in session"))
			return
		}
		var in struct {
			Title          *string                `json:"title,omitempty"`
			Goal           *string                `json:"goal,omitempty"`
			Outcome        *string                `json:"outcome,omitempty"`
			OutcomeSummary *string                `json:"outcome_summary,omitempty"`
			ResultPreview  *string                `json:"result_preview,omitempty"`
			FailureReason  *string                `json:"failure_reason,omitempty"`
			Importance     *float64               `json:"importance,omitempty"`
			Confidence     *float64               `json:"confidence,omitempty"`
			UserFeedback   *string                `json:"user_feedback,omitempty"`
			CriticScore    *float64               `json:"critic_score,omitempty"`
			Metadata       map[string]any         `json:"metadata,omitempty"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		ep := current.Episode
		if in.Title != nil {
			ep.Title = *in.Title
		}
		if in.Goal != nil {
			ep.Goal = *in.Goal
		}
		if in.Outcome != nil {
			ep.Outcome = *in.Outcome
		}
		if in.OutcomeSummary != nil {
			ep.OutcomeSummary = *in.OutcomeSummary
		}
		if in.ResultPreview != nil {
			ep.ResultPreview = *in.ResultPreview
		}
		if in.FailureReason != nil {
			ep.FailureReason = *in.FailureReason
		}
		if in.Importance != nil {
			ep.Importance = clampFloat(*in.Importance, 0, 1)
		}
		if in.Confidence != nil {
			ep.Confidence = clampFloat(*in.Confidence, 0, 1)
		}
		if in.UserFeedback != nil {
			ep.UserFeedback = *in.UserFeedback
		}
		if in.CriticScore != nil {
			ep.CriticScore = *in.CriticScore
		}
		if in.Metadata != nil {
			ep.MetadataJSON = encodeJSONOrEmpty(in.Metadata)
		}
		updated, err := svc.UpdateEpisode(r.Context(), ep)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := svc.DeleteEpisode(r.Context(), episodeID); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l2.delete_episode", "memory_episodes", episodeID, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleL2EpisodeAction(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID, episodeID, action string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	switch action {
	case "reindex":
		if err := svc.BuildIndexFor(r.Context(), episodeID); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l2.reindex", "memory_episodes", episodeID, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusAccepted)
	case "consolidate":
		// Phase 3: the consolidation worker will pick the episode up. For
		// now we just bump the audit trail so the front-end can confirm
		// the request reached the backend.
		_ = h.auditSvc.Log("l2.consolidate_request", "memory_episodes", episodeID, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusAccepted)
	default:
		writeErr(w, http.StatusNotFound, errors.New("unknown episode action"))
	}
	_ = sessionID
}

// --- Marks ------------------------------------------------------------------

func (h *HTTPHandler) handleL2Marks(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID string, parts []string) {
	switch len(parts) {
	case 0:
		switch r.Method {
		case http.MethodGet:
			markType := r.URL.Query().Get("type")
			limit := parsePositiveInt(r.URL.Query().Get("limit"), 100)
			marks, err := svc.ListMarks(r.Context(), sessionID, markType, limit)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			if marks == nil {
				marks = []domain.MemoryEventMark{}
			}
			writeJSON(w, http.StatusOK, listResponse[domain.MemoryEventMark]{Items: marks})
		case http.MethodPost:
			var in service.MarkInput
			if !decodeBody(w, r, &in) {
				return
			}
			stored, err := svc.Mark(r.Context(), in)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			_ = h.auditSvc.Log("l2.mark", "memory_event_marks", stored.ID, r.Header.Get("X-Request-Id"), stored.MarkType)
			writeJSON(w, http.StatusCreated, stored)
		default:
			methodNotAllowed(w)
		}
	case 1:
		if r.Method != http.MethodDelete {
			methodNotAllowed(w)
			return
		}
		if err := svc.UnMark(r.Context(), parts[0]); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeErr(w, http.StatusNotFound, errors.New("unknown marks path"))
	}
}

// --- Recall -----------------------------------------------------------------

func (h *HTTPHandler) handleL2Recall(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in domain.MemoryL2RecallQuery
	if !decodeBody(w, r, &in) {
		return
	}
	in.SessionID = sessionID
	results, err := svc.RecallByQuery(r.Context(), in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if results == nil {
		results = []domain.MemoryL2RecallResult{}
	}
	writeJSON(w, http.StatusOK, listResponse[domain.MemoryL2RecallResult]{Items: results})
}

// --- Admin ------------------------------------------------------------------

func (h *HTTPHandler) handleL2RetentionRun(w http.ResponseWriter, r *http.Request) {
	svc := h.l2Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L2 service is not configured"))
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	report, err := svc.ApplyRetention(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// --- helpers ----------------------------------------------------------------

// l2Service is a thin accessor that hides the chatSvc indirection. Returning
// nil triggers a 503 in callers so misconfigured builds (no MemoryL2Service
// injected) don't panic.
func (h *HTTPHandler) l2Service() *service.MemoryL2Service {
	if h.chatSvc == nil {
		return nil
	}
	return h.chatSvc.MemoryL2()
}

func splitCSV(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func encodeJSONOrEmpty(value any) string {
	if value == nil {
		return "{}"
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}
