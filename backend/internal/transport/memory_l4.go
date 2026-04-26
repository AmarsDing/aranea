// transport/memory_l4.go exposes the L4 persistent / knowledge-graph
// HTTP surface described in `aranea/docs/16 memory-L4-persistent.md`
// §6.2 (entities), §6.3 (relations), and §6.5 (admin / extraction).
// Resources are workspace- / user- / agent-scoped (not session-scoped)
// so the routes are registered in handler.go directly under
// `/api/v1/memory/l4/...`.
package transport

import (
	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
	"arenea/backend/internal/service"
)

// registerMemoryL4Routes installs the user-facing entity / relation /
// neighborhood endpoints and the admin-only extraction / stats
// endpoints. Admin endpoints live under /api/v1/admin/memory/l4/.
func (h *HTTPHandler) registerMemoryL4Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/memory/l4/entities", h.handleL4EntitiesCollection)
	mux.HandleFunc("/api/v1/memory/l4/entities/", h.handleL4EntitiesItem)
	mux.HandleFunc("/api/v1/memory/l4/relations", h.handleL4RelationsCollection)
	mux.HandleFunc("/api/v1/memory/l4/relations/", h.handleL4RelationsItem)
	mux.HandleFunc("/api/v1/memory/l4/neighborhood", h.handleL4Neighborhood)
	mux.HandleFunc("/api/v1/memory/l4/search", h.handleL4Search)

	mux.HandleFunc("/api/v1/admin/memory/l4/extract/episode", h.handleL4ExtractEpisode)
	mux.HandleFunc("/api/v1/admin/memory/l4/extract/fact", h.handleL4ExtractFact)
}

// l4Service mirrors l3Service: nil triggers a 503 in callers so a
// misconfigured build (no MemoryL4Service injected) surfaces clearly.
func (h *HTTPHandler) l4Service() *service.MemoryL4Service {
	if h.chatSvc == nil {
		return nil
	}
	return h.chatSvc.MemoryL4()
}

// --- Entities --------------------------------------------------------------

func (h *HTTPHandler) handleL4EntitiesCollection(w http.ResponseWriter, r *http.Request) {
	svc := h.l4Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := repository.EntityListQuery{
			ScopeType:   domain.ScopeType(r.URL.Query().Get("scope_type")),
			ScopeID:     r.URL.Query().Get("scope_id"),
			WorkspaceID: r.URL.Query().Get("workspace_id"),
			UserID:      r.URL.Query().Get("user_id"),
			EntityType:  domain.EntityType(r.URL.Query().Get("entity_type")),
			Status:      r.URL.Query().Get("status"),
			Keyword:     r.URL.Query().Get("keyword"),
			Limit:       parsePositiveInt(r.URL.Query().Get("limit"), 50),
			Offset:      parsePositiveInt(r.URL.Query().Get("offset"), 0),
		}
		out, err := svc.ListEntities(r.Context(), q)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var in service.EntityUpsertInput
		if !decodeBody(w, r, &in) {
			return
		}
		ent, err := svc.UpsertEntity(r.Context(), in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l4.upsert_entity", "memory_entities", ent.ID, r.Header.Get("X-Request-Id"), string(ent.ScopeType))
		writeJSON(w, http.StatusCreated, ent)
	default:
		methodNotAllowed(w)
	}
}

// handleL4EntitiesItem dispatches /api/v1/memory/l4/entities/{id}[/...]
// paths. Sub-resources:
//   - /versions  (GET)
//   - /facts     (GET)
//   - /rename    (POST)
//   - /merge     (POST)
//   - /archive   (POST)
func (h *HTTPHandler) handleL4EntitiesItem(w http.ResponseWriter, r *http.Request) {
	svc := h.l4Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/memory/l4/entities/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, errors.New("entity id is required"))
		return
	}
	entityID := parts[0]
	if len(parts) == 1 {
		h.handleL4EntityItemRoot(w, r, svc, entityID)
		return
	}
	switch parts[1] {
	case "versions":
		h.handleL4EntityVersions(w, r, svc, entityID)
	case "facts":
		h.handleL4EntityFacts(w, r, svc, entityID)
	case "rename":
		h.handleL4EntityRename(w, r, svc, entityID)
	case "merge":
		h.handleL4EntityMerge(w, r, svc, entityID)
	case "archive":
		h.handleL4EntityArchive(w, r, svc, entityID)
	default:
		writeErr(w, http.StatusNotFound, errors.New("unknown entity sub-resource"))
	}
}

func (h *HTTPHandler) handleL4EntityItemRoot(w http.ResponseWriter, r *http.Request, svc *service.MemoryL4Service, id string) {
	switch r.Method {
	case http.MethodGet:
		ent, err := svc.GetEntity(r.Context(), id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, ent)
	case http.MethodPatch:
		var in service.EntityUpsertInput
		if !decodeBody(w, r, &in) {
			return
		}
		in.ID = id
		ent, err := svc.UpsertEntity(r.Context(), in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l4.update_entity", "memory_entities", ent.ID, r.Header.Get("X-Request-Id"), in.Reason)
		writeJSON(w, http.StatusOK, ent)
	case http.MethodDelete:
		by := r.URL.Query().Get("by")
		reason := r.URL.Query().Get("reason")
		if err := svc.DeleteEntity(r.Context(), id, by, reason); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l4.delete_entity", "memory_entities", id, r.Header.Get("X-Request-Id"), by)
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleL4EntityVersions(w http.ResponseWriter, r *http.Request, svc *service.MemoryL4Service, id string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
	versions, err := svc.ListEntityVersions(r.Context(), id, limit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if versions == nil {
		versions = []domain.MemoryEntityVersion{}
	}
	writeJSON(w, http.StatusOK, listResponse[domain.MemoryEntityVersion]{Items: versions})
}

func (h *HTTPHandler) handleL4EntityFacts(w http.ResponseWriter, r *http.Request, svc *service.MemoryL4Service, id string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
	links, err := svc.ListEntityFacts(r.Context(), id, limit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if links == nil {
		links = []domain.MemoryEntityFactLink{}
	}
	writeJSON(w, http.StatusOK, listResponse[domain.MemoryEntityFactLink]{Items: links})
}

func (h *HTTPHandler) handleL4EntityRename(w http.ResponseWriter, r *http.Request, svc *service.MemoryL4Service, id string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		Name   string `json:"name"`
		By     string `json:"by"`
		Reason string `json:"reason"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	ent, err := svc.RenameEntity(r.Context(), id, in.Name, in.By, in.Reason)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("l4.rename_entity", "memory_entities", id, r.Header.Get("X-Request-Id"), in.Reason)
	writeJSON(w, http.StatusOK, ent)
}

func (h *HTTPHandler) handleL4EntityMerge(w http.ResponseWriter, r *http.Request, svc *service.MemoryL4Service, primaryID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		Sources []string `json:"sources"`
		By      string   `json:"by"`
		Reason  string   `json:"reason"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	if err := svc.MergeEntities(r.Context(), primaryID, in.Sources, in.By, in.Reason); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("l4.merge_entities", "memory_entities", primaryID, r.Header.Get("X-Request-Id"), in.Reason)
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) handleL4EntityArchive(w http.ResponseWriter, r *http.Request, svc *service.MemoryL4Service, id string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		By     string `json:"by"`
		Reason string `json:"reason"`
	}
	_ = decodeBody(w, r, &in) // optional body
	if err := svc.ArchiveEntity(r.Context(), id, in.By, in.Reason); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("l4.archive_entity", "memory_entities", id, r.Header.Get("X-Request-Id"), in.Reason)
	w.WriteHeader(http.StatusNoContent)
}

// --- Relations --------------------------------------------------------------

func (h *HTTPHandler) handleL4RelationsCollection(w http.ResponseWriter, r *http.Request) {
	svc := h.l4Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	switch r.Method {
	case http.MethodPost:
		var in service.RelationUpsertInput
		if !decodeBody(w, r, &in) {
			return
		}
		rel, err := svc.UpsertRelation(r.Context(), in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l4.upsert_relation", "memory_relations", rel.ID, r.Header.Get("X-Request-Id"), string(rel.RelationType))
		writeJSON(w, http.StatusCreated, rel)
	case http.MethodGet:
		nodeID := r.URL.Query().Get("node_id")
		if nodeID == "" {
			writeErr(w, http.StatusBadRequest, errors.New("node_id is required"))
			return
		}
		limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
		rels, err := svc.ListRelationsForNode(r.Context(), nodeID, limit)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if rels == nil {
			rels = []domain.MemoryRelation{}
		}
		writeJSON(w, http.StatusOK, listResponse[domain.MemoryRelation]{Items: rels})
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleL4RelationsItem(w http.ResponseWriter, r *http.Request) {
	svc := h.l4Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/memory/l4/relations/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeErr(w, http.StatusBadRequest, errors.New("relation id is required"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		rel, err := svc.GetRelation(r.Context(), id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, rel)
	case http.MethodDelete:
		by := r.URL.Query().Get("by")
		reason := r.URL.Query().Get("reason")
		if err := svc.DeleteRelation(r.Context(), id, by, reason); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l4.delete_relation", "memory_relations", id, r.Header.Get("X-Request-Id"), by)
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

// --- Neighborhood / search --------------------------------------------------

func (h *HTTPHandler) handleL4Neighborhood(w http.ResponseWriter, r *http.Request) {
	svc := h.l4Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	centerID := r.URL.Query().Get("center_id")
	hops := parsePositiveInt(r.URL.Query().Get("hops"), 1)
	maxNodes := parsePositiveInt(r.URL.Query().Get("max_nodes"), 12)
	nb, err := svc.Neighborhood(r.Context(), centerID, hops, maxNodes)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, nb)
}

func (h *HTTPHandler) handleL4Search(w http.ResponseWriter, r *http.Request) {
	svc := h.l4Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	scope := domain.ScopeType(r.URL.Query().Get("scope_type"))
	scopeID := r.URL.Query().Get("scope_id")
	query := r.URL.Query().Get("q")
	topK := parsePositiveInt(r.URL.Query().Get("top_k"), 10)
	hits, err := svc.SearchByText(r.Context(), scope, scopeID, query, topK)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if hits == nil {
		hits = []domain.MemoryEntity{}
	}
	writeJSON(w, http.StatusOK, listResponse[domain.MemoryEntity]{Items: hits})
}

// --- Admin / extraction -----------------------------------------------------

func (h *HTTPHandler) handleL4ExtractEpisode(w http.ResponseWriter, r *http.Request) {
	svc := h.l4Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		EpisodeID string `json:"episode_id"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	report, err := svc.ExtractFromEpisode(r.Context(), in.EpisodeID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("l4.extract_episode", "memory_entities", in.EpisodeID, r.Header.Get("X-Request-Id"), "")
	writeJSON(w, http.StatusOK, report)
}

func (h *HTTPHandler) handleL4ExtractFact(w http.ResponseWriter, r *http.Request) {
	svc := h.l4Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L4 service is not configured"))
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
	report, err := svc.ExtractFromFact(r.Context(), in.FactID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("l4.extract_fact", "memory_entities", in.FactID, r.Header.Get("X-Request-Id"), "")
	writeJSON(w, http.StatusOK, report)
}
