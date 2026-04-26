// transport/agent_evolution.go exposes the L4 self-evolution HTTP
// surface described in `aranea/docs/16 memory-L4-persistent.md` §6.4.
// All routes live under `/api/v1/agents/{agent_id}/evolution/...` and
// `/api/v1/admin/agents/{agent_id}/evolution/...` so the agent context
// is unambiguous.
package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/service"
)

// registerAgentEvolutionRoutes installs every evolution endpoint at the
// top level. The dispatch function does the agent-id parsing.
func (h *HTTPHandler) registerAgentEvolutionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/agent-evolution/", h.handleAgentEvolution)
}

// evolutionService mirrors l3Service / l4Service: nil triggers a 503 in
// callers so a misconfigured build (no AgentEvolutionService injected)
// surfaces clearly.
func (h *HTTPHandler) evolutionService() *service.AgentEvolutionService {
	if h.chatSvc == nil {
		return nil
	}
	return h.chatSvc.AgentEvolution()
}

// handleAgentEvolution dispatches /api/v1/agent-evolution/{agent_id}/...
// paths. The supported sub-resources are:
//   - identity         (GET, PATCH)
//   - strategy         (GET, PATCH)
//   - proposals        (GET, POST)
//   - proposals/{id}/approve|reject (POST)
//   - events           (GET)
//   - events/{id}/revert (POST)
//   - skill-stats      (GET)
//   - scan             (POST)
func (h *HTTPHandler) handleAgentEvolution(w http.ResponseWriter, r *http.Request) {
	svc := h.evolutionService()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("agent evolution service is not configured"))
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/agent-evolution/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, errors.New("agent_id and sub-resource are required"))
		return
	}
	agentID := parts[0]
	switch parts[1] {
	case "identity":
		h.handleAgentIdentity(w, r, svc, agentID)
	case "strategy":
		h.handleAgentStrategy(w, r, svc, agentID)
	case "proposals":
		h.handleAgentProposals(w, r, svc, agentID, parts[2:])
	case "events":
		h.handleAgentEvents(w, r, svc, agentID, parts[2:])
	case "skill-stats":
		h.handleAgentSkillStats(w, r, svc, agentID)
	case "scan":
		h.handleAgentEvolutionScan(w, r, svc, agentID)
	case "metrics":
		h.handleAgentEvolutionMetrics(w, r, svc, agentID)
	case "suggestions":
		h.handleAgentEvolutionSuggestions(w, r, svc, agentID)
	case "training-data":
		h.handleAgentEvolutionTrainingData(w, r, svc, agentID)
	default:
		writeErr(w, http.StatusNotFound, errors.New("unknown evolution sub-resource"))
	}
}

// handleAgentEvolutionAgentPath dispatches the spec-compatible
// /api/v1/agents/{id}/identity, /strategy, and /evolution/... aliases while
// keeping /api/v1/agent-evolution/{id}/... backwards-compatible.
func (h *HTTPHandler) handleAgentEvolutionAgentPath(w http.ResponseWriter, r *http.Request, pathSuffix string) bool {
	svc := h.evolutionService()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("agent evolution service is not configured"))
		return true
	}
	parts := strings.Split(strings.Trim(pathSuffix, "/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		return false
	}
	agentID := parts[0]
	switch parts[1] {
	case "identity":
		h.handleAgentIdentity(w, r, svc, agentID)
	case "strategy":
		h.handleAgentStrategy(w, r, svc, agentID)
	case "skill-stats":
		h.handleAgentSkillStats(w, r, svc, agentID)
	case "evolution":
		if len(parts) < 3 {
			writeErr(w, http.StatusBadRequest, errors.New("evolution sub-resource is required"))
			return true
		}
		switch parts[2] {
		case "events":
			h.handleAgentEvents(w, r, svc, agentID, parts[3:])
		case "proposals":
			h.handleAgentProposals(w, r, svc, agentID, parts[3:])
		case "scan":
			h.handleAgentEvolutionScan(w, r, svc, agentID)
		case "metrics":
			h.handleAgentEvolutionMetrics(w, r, svc, agentID)
		case "suggestions":
			h.handleAgentEvolutionSuggestions(w, r, svc, agentID)
		case "training-data":
			h.handleAgentEvolutionTrainingData(w, r, svc, agentID)
		default:
			writeErr(w, http.StatusNotFound, errors.New("unknown evolution sub-resource"))
		}
	default:
		return false
	}
	return true
}

// --- Identity ---------------------------------------------------------------

func (h *HTTPHandler) handleAgentIdentity(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	switch r.Method {
	case http.MethodGet:
		identity, err := svc.GetIdentity(r.Context(), agentID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, identity)
	case http.MethodPatch:
		var patch service.IdentityPatch
		if !decodeBody(w, r, &patch) {
			return
		}
		updated, err := svc.UpdateIdentity(r.Context(), agentID, patch)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("agent.evolution.identity.update", "agent_identity", agentID, r.Header.Get("X-Request-Id"), patch.Reason)
		writeJSON(w, http.StatusOK, updated)
	default:
		methodNotAllowed(w)
	}
}

// --- Strategy ---------------------------------------------------------------

func (h *HTTPHandler) handleAgentStrategy(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	switch r.Method {
	case http.MethodGet:
		profile, err := svc.GetStrategy(r.Context(), agentID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, profile)
	case http.MethodPatch:
		var patch service.StrategyPatch
		if !decodeBody(w, r, &patch) {
			return
		}
		updated, err := svc.UpdateStrategy(r.Context(), agentID, patch)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("agent.evolution.strategy.update", "agent_strategy_profile", agentID, r.Header.Get("X-Request-Id"), patch.Reason)
		writeJSON(w, http.StatusOK, updated)
	default:
		methodNotAllowed(w)
	}
}

// --- Proposals --------------------------------------------------------------

func (h *HTTPHandler) handleAgentProposals(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string, tail []string) {
	if len(tail) == 0 {
		switch r.Method {
		case http.MethodGet:
			status := r.URL.Query().Get("status")
			limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
			offset := parsePositiveInt(r.URL.Query().Get("offset"), 0)
			out, err := svc.ListProposals(r.Context(), agentID, status, limit, offset)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, out)
		case http.MethodPost:
			var in service.ProposalInput
			if !decodeBody(w, r, &in) {
				return
			}
			in.AgentID = agentID
			prop, err := svc.Propose(r.Context(), in)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			_ = h.auditSvc.Log("agent.evolution.proposal.create", "agent_evolution_proposals", prop.ID, r.Header.Get("X-Request-Id"), in.Source)
			writeJSON(w, http.StatusCreated, prop)
		default:
			methodNotAllowed(w)
		}
		return
	}
	if tail[0] == "" {
		writeErr(w, http.StatusBadRequest, errors.New("proposal id is required"))
		return
	}
	proposalID := tail[0]
	if len(tail) == 1 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		prop, err := svc.GetProposal(r.Context(), proposalID)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, prop)
		return
	}
	switch tail[1] {
	case "approve":
		h.handleProposalApprove(w, r, svc, proposalID)
	case "reject":
		h.handleProposalReject(w, r, svc, proposalID)
	default:
		writeErr(w, http.StatusNotFound, errors.New("unknown proposal action"))
	}
}

func (h *HTTPHandler) handleProposalApprove(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, proposalID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		By string `json:"by"`
	}
	_ = decodeBody(w, r, &in)
	event, err := svc.Approve(r.Context(), proposalID, in.By)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("agent.evolution.proposal.approve", "agent_evolution_proposals", proposalID, r.Header.Get("X-Request-Id"), in.By)
	writeJSON(w, http.StatusOK, event)
}

func (h *HTTPHandler) handleProposalReject(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, proposalID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		By     string `json:"by"`
		Reason string `json:"reason"`
	}
	_ = decodeBody(w, r, &in)
	if err := svc.Reject(r.Context(), proposalID, in.By, in.Reason); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("agent.evolution.proposal.reject", "agent_evolution_proposals", proposalID, r.Header.Get("X-Request-Id"), in.Reason)
	w.WriteHeader(http.StatusNoContent)
}

// --- Events -----------------------------------------------------------------

func (h *HTTPHandler) handleAgentEvents(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string, tail []string) {
	if len(tail) == 0 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		kind := r.URL.Query().Get("kind")
		limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
		offset := parsePositiveInt(r.URL.Query().Get("offset"), 0)
		out, err := svc.ListEvents(r.Context(), agentID, kind, limit, offset)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	if tail[0] == "" {
		writeErr(w, http.StatusBadRequest, errors.New("event id is required"))
		return
	}
	eventID := tail[0]
	if len(tail) == 1 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		event, err := svc.GetEvent(r.Context(), eventID)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, event)
		return
	}
	switch tail[1] {
	case "revert":
		h.handleEventRevert(w, r, svc, eventID)
	default:
		writeErr(w, http.StatusNotFound, errors.New("unknown event action"))
	}
}

func (h *HTTPHandler) handleEventRevert(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, eventID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		By     string `json:"by"`
		Reason string `json:"reason"`
	}
	_ = decodeBody(w, r, &in)
	event, err := svc.Revert(r.Context(), eventID, in.By, in.Reason)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("agent.evolution.event.revert", "agent_evolution_events", eventID, r.Header.Get("X-Request-Id"), in.Reason)
	writeJSON(w, http.StatusOK, event)
}

// --- Skill stats ------------------------------------------------------------

func (h *HTTPHandler) handleAgentSkillStats(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	switch r.Method {
	case http.MethodGet:
		limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
		stats, err := svc.ListSkillStats(r.Context(), agentID, limit)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if stats == nil {
			stats = []domain.AgentSkillStat{}
		}
		writeJSON(w, http.StatusOK, listResponse[domain.AgentSkillStat]{Items: stats})
	case http.MethodPost:
		var in domain.AgentSkillStat
		if !decodeBody(w, r, &in) {
			return
		}
		in.AgentID = agentID
		stat, err := svc.UpsertSkillStat(r.Context(), in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, stat)
	default:
		methodNotAllowed(w)
	}
}

// --- Worker -----------------------------------------------------------------

func (h *HTTPHandler) handleAgentEvolutionScan(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	report, err := svc.RunEvolutionScan(r.Context(), agentID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("agent.evolution.scan", "agent_identity", agentID, r.Header.Get("X-Request-Id"), report.Note)
	writeJSON(w, http.StatusOK, report)
}

func (h *HTTPHandler) handleAgentEvolutionMetrics(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	report, err := svc.Metrics(r.Context(), agentID, r.URL.Query().Get("range"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *HTTPHandler) handleAgentEvolutionSuggestions(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 20)
	items, err := svc.Suggestions(r.Context(), agentID, r.URL.Query().Get("range"), limit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if items == nil {
		items = []service.EvolutionSuggestion{}
	}
	writeJSON(w, http.StatusOK, listResponse[service.EvolutionSuggestion]{Items: items})
}

func (h *HTTPHandler) handleAgentEvolutionTrainingData(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 100)
	items, err := svc.TrainingData(r.Context(), agentID, limit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if r.URL.Query().Get("format") == "jsonl" {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		enc := json.NewEncoder(w)
		for _, item := range items {
			_ = enc.Encode(item)
		}
		return
	}
	if items == nil {
		items = []service.EvolutionTrainingExample{}
	}
	writeJSON(w, http.StatusOK, listResponse[service.EvolutionTrainingExample]{Items: items})
}
