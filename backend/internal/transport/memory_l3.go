// transport/memory_l3.go exposes the L3 semantic-memory HTTP surface
// described in `aranea/docs/15 memory-L3-semantic.md` §6.2 – §6.6. The
// resource is workspace-/user-/agent-scoped (not session-scoped) so the
// routes are registered in handler.go directly under
// `/api/v1/memory/l3/...` rather than dispatched from sessions.go.
package transport

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
	"arenea/backend/internal/service"
)

// registerMemoryL3Routes installs both the user-facing fact / recall /
// feedback endpoints and the admin-only decay / embedding / stats
// endpoints. Admin endpoints live under /api/v1/admin/memory/l3/.
func (h *HTTPHandler) registerMemoryL3Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/memory/l3/facts", h.handleL3FactsCollection)
	mux.HandleFunc("/api/v1/memory/l3/facts/", h.handleL3FactsItem)
	mux.HandleFunc("/api/v1/memory/l3/facts:bulk-upsert", h.handleL3FactsBulkUpsert)
	mux.HandleFunc("/api/v1/memory/l3/recall", h.handleL3Recall)
	mux.HandleFunc("/api/v1/memory/l3/conflicts", h.handleL3Conflicts)
	mux.HandleFunc("/api/v1/memory/l3/conflicts/", h.handleL3ConflictItem)

	mux.HandleFunc("/api/v1/admin/memory/l3/decay/run", h.handleL3DecayRun)
	mux.HandleFunc("/api/v1/admin/memory/l3/embedding/rebuild", h.handleL3EmbeddingRebuild)
	mux.HandleFunc("/api/v1/admin/memory/l3/stats", h.handleL3Stats)
}

// l3Service mirrors l2Service: nil triggers a 503 in callers so a
// misconfigured build (no MemoryL3Service injected) surfaces clearly.
func (h *HTTPHandler) l3Service() *service.MemoryL3Service {
	if h.chatSvc == nil {
		return nil
	}
	return h.chatSvc.MemoryL3()
}

// --- Fact CRUD --------------------------------------------------------------

func (h *HTTPHandler) handleL3FactsCollection(w http.ResponseWriter, r *http.Request) {
	svc := h.l3Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := repository.FactListQuery{
			ScopeType:   domain.ScopeType(r.URL.Query().Get("scope_type")),
			ScopeID:     r.URL.Query().Get("scope_id"),
			WorkspaceID: r.URL.Query().Get("workspace_id"),
			UserID:      r.URL.Query().Get("user_id"),
			TeamID:      r.URL.Query().Get("team_id"),
			AgentID:     r.URL.Query().Get("agent_id"),
			Status:      r.URL.Query().Get("status"),
			Kind:        domain.FactKind(r.URL.Query().Get("kind")),
			Tags:        splitCSV(r.URL.Query().Get("tags")),
			Keyword:     r.URL.Query().Get("keyword"),
			Limit:       parsePositiveInt(r.URL.Query().Get("limit"), 20),
			Offset:      parsePositiveInt(r.URL.Query().Get("offset"), 0),
		}
		out, err := svc.List(r.Context(), q)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var in domain.FactUpsertInput
		if !decodeBody(w, r, &in) {
			return
		}
		fact, err := svc.UpsertFact(r.Context(), in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l3.upsert_fact", "memory_facts", fact.ID, r.Header.Get("X-Request-Id"), string(fact.ScopeType))
		writeJSON(w, http.StatusCreated, fact)
	default:
		methodNotAllowed(w)
	}
}

// handleL3FactsItem dispatches /api/v1/memory/l3/facts/{id}[/...] paths.
// Sub-resources:
//   - /versions  (GET)
//   - /feedback  (GET, POST)
//   - /rollback  (POST)
func (h *HTTPHandler) handleL3FactsItem(w http.ResponseWriter, r *http.Request) {
	svc := h.l3Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/memory/l3/facts/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, errors.New("fact id is required"))
		return
	}
	factID := parts[0]
	if len(parts) == 1 {
		h.handleL3FactItemRoot(w, r, svc, factID)
		return
	}
	switch parts[1] {
	case "versions":
		h.handleL3FactVersions(w, r, svc, factID)
	case "feedback":
		h.handleL3FactFeedback(w, r, svc, factID)
	case "rollback":
		h.handleL3FactRollback(w, r, svc, factID)
	default:
		writeErr(w, http.StatusNotFound, errors.New("unknown fact sub-resource"))
	}
}

func (h *HTTPHandler) handleL3FactItemRoot(w http.ResponseWriter, r *http.Request, svc *service.MemoryL3Service, factID string) {
	switch r.Method {
	case http.MethodGet:
		fact, err := svc.Get(r.Context(), factID)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, fact)
	case http.MethodPatch:
		var patch service.FactPatch
		if !decodeBody(w, r, &patch) {
			return
		}
		updated, err := svc.UpdateFact(r.Context(), factID, patch)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l3.update_fact", "memory_facts", factID, r.Header.Get("X-Request-Id"), patch.Reason)
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		by := r.URL.Query().Get("by")
		if err := svc.DeleteFact(r.Context(), factID, by); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l3.delete_fact", "memory_facts", factID, r.Header.Get("X-Request-Id"), by)
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleL3FactVersions(w http.ResponseWriter, r *http.Request, svc *service.MemoryL3Service, factID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
	versions, err := svc.ListVersions(r.Context(), factID, limit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if versions == nil {
		versions = []domain.FactVersion{}
	}
	writeJSON(w, http.StatusOK, listResponse[domain.FactVersion]{Items: versions})
}

func (h *HTTPHandler) handleL3FactFeedback(w http.ResponseWriter, r *http.Request, svc *service.MemoryL3Service, factID string) {
	switch r.Method {
	case http.MethodGet:
		limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
		fbs, err := svc.ListFeedback(r.Context(), factID, limit)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if fbs == nil {
			fbs = []domain.FactFeedback{}
		}
		writeJSON(w, http.StatusOK, listResponse[domain.FactFeedback]{Items: fbs})
	case http.MethodPost:
		var in domain.FactFeedback
		if !decodeBody(w, r, &in) {
			return
		}
		in.FactID = factID
		if err := svc.Feedback(r.Context(), in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l3.feedback", "memory_facts", factID, r.Header.Get("X-Request-Id"), in.Type)
		w.WriteHeader(http.StatusAccepted)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleL3FactRollback(w http.ResponseWriter, r *http.Request, svc *service.MemoryL3Service, factID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		ToVersion int    `json:"to_version"`
		By        string `json:"by"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	updated, err := svc.RollbackFact(r.Context(), factID, in.ToVersion, in.By)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("l3.rollback_fact", "memory_facts", factID, r.Header.Get("X-Request-Id"), strconv.Itoa(in.ToVersion))
	writeJSON(w, http.StatusOK, updated)
}

func (h *HTTPHandler) handleL3FactsBulkUpsert(w http.ResponseWriter, r *http.Request) {
	svc := h.l3Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		Items []domain.FactUpsertInput `json:"items"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	if len(in.Items) == 0 {
		writeErr(w, http.StatusBadRequest, errors.New("items is required"))
		return
	}
	report, err := svc.BulkUpsert(r.Context(), in.Items)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("l3.bulk_upsert", "memory_facts", "", r.Header.Get("X-Request-Id"), strconv.Itoa(len(in.Items)))
	writeJSON(w, http.StatusOK, report)
}

// --- Recall -----------------------------------------------------------------

func (h *HTTPHandler) handleL3Recall(w http.ResponseWriter, r *http.Request) {
	svc := h.l3Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in domain.FactRecallQuery
	if !decodeBody(w, r, &in) {
		return
	}
	hits, err := svc.Recall(r.Context(), in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if hits == nil {
		hits = []domain.FactRecallHit{}
	}
	writeJSON(w, http.StatusOK, listResponse[domain.FactRecallHit]{Items: hits})
}

// --- Conflicts --------------------------------------------------------------

func (h *HTTPHandler) handleL3Conflicts(w http.ResponseWriter, r *http.Request) {
	svc := h.l3Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	scope := domain.ScopeType(r.URL.Query().Get("scope_type"))
	scopeID := r.URL.Query().Get("scope_id")
	conflicts, err := svc.ListOpenConflicts(r.Context(), scope, scopeID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if conflicts == nil {
		conflicts = []domain.FactConflict{}
	}
	writeJSON(w, http.StatusOK, listResponse[domain.FactConflict]{Items: conflicts})
}

func (h *HTTPHandler) handleL3ConflictItem(w http.ResponseWriter, r *http.Request) {
	svc := h.l3Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/memory/l3/conflicts/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, errors.New("conflict id is required"))
		return
	}
	conflictID := parts[0]
	if len(parts) == 2 && parts[1] == "resolve" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var in struct {
			Resolution string `json:"resolution"`
			By         string `json:"by"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		if err := svc.ResolveConflict(r.Context(), conflictID, in.Resolution, in.By); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l3.resolve_conflict", "memory_fact_conflicts", conflictID, r.Header.Get("X-Request-Id"), in.Resolution)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeErr(w, http.StatusNotFound, errors.New("unknown conflict path"))
}

// --- Admin ------------------------------------------------------------------

func (h *HTTPHandler) handleL3DecayRun(w http.ResponseWriter, r *http.Request) {
	svc := h.l3Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	report, err := svc.RunDecayBatch(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	_ = h.auditSvc.Log("l3.decay_run", "memory_facts", "", r.Header.Get("X-Request-Id"), "")
	writeJSON(w, http.StatusOK, report)
}

func (h *HTTPHandler) handleL3EmbeddingRebuild(w http.ResponseWriter, r *http.Request) {
	svc := h.l3Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		FactID string `json:"fact_id"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	if err := svc.BuildEmbedding(r.Context(), in.FactID); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("l3.embedding_rebuild", "memory_facts", in.FactID, r.Header.Get("X-Request-Id"), "")
	w.WriteHeader(http.StatusAccepted)
}

func (h *HTTPHandler) handleL3Stats(w http.ResponseWriter, r *http.Request) {
	svc := h.l3Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L3 service is not configured"))
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	scope := domain.ScopeType(r.URL.Query().Get("scope_type"))
	scopeID := r.URL.Query().Get("scope_id")
	report, err := svc.Stats(r.Context(), scope, scopeID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
