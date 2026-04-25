// transport/memory_l1.go exposes the L1 working-memory HTTP surface
// described in `aranea/docs/13 memory-L1-working.md` §6.2 and §6.3. The
// router lives in sessions.go (handleSessionByID) which forwards L1 paths
// here. Schema-management routes (§6.2) live on /api/v1/memory/l1/schemas
// and are wired separately by registerMemoryL1Routes.
package transport

import (
	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/service"
)

// registerMemoryL1Routes is called from registerRoutes to bind the
// agent-scoped schema management endpoints. Session-scoped task / field
// routes are dispatched from handleSessionByID via splitSessionPathSuffix.
func (h *HTTPHandler) registerMemoryL1Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/memory/l1/schemas", h.handleL1Schemas)
	mux.HandleFunc("/api/v1/memory/l1/schemas/", h.handleL1SchemaByID)
}

// handleL1Routes dispatches /api/v1/sessions/{sid}/l1/... requests. The
// suffix has already been parsed into segments (e.g. "tasks/abc/fields/x").
func (h *HTTPHandler) handleL1Routes(w http.ResponseWriter, r *http.Request, sessionID, suffix string) {
	svc := h.l1Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L1 service is not configured"))
		return
	}
	parts := strings.Split(suffix, "/")
	if len(parts) == 0 || parts[0] != "tasks" {
		writeErr(w, http.StatusNotFound, errors.New("unknown l1 path"))
		return
	}
	switch len(parts) {
	case 1:
		h.handleL1TasksCollection(w, r, svc, sessionID)
	case 2:
		h.handleL1TaskItem(w, r, svc, sessionID, parts[1])
	default:
		h.handleL1TaskSubresource(w, r, svc, sessionID, parts[1], parts[2:])
	}
}

func (h *HTTPHandler) handleL1TasksCollection(w http.ResponseWriter, r *http.Request, svc *service.MemoryL1Service, sessionID string) {
	switch r.Method {
	case http.MethodGet:
		query := domain.L1TaskListQuery{
			SessionID:    sessionID,
			AgentID:      r.URL.Query().Get("agent_id"),
			Status:       r.URL.Query().Get("status"),
			IncludeEnded: r.URL.Query().Get("include_ended") == "true",
		}
		tasks, err := svc.ListTasks(r.Context(), query)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse[domain.MemoryL1Task]{Items: tasks})
	case http.MethodPost:
		var in struct {
			RunID        string                `json:"run_id"`
			TeamID       string                `json:"team_id"`
			AgentID      string                `json:"agent_id"`
			TaskKey      string                `json:"task_key"`
			TaskTitle    string                `json:"task_title"`
			TaskGoal     string                `json:"task_goal"`
			ParentTaskID string                `json:"parent_task_id"`
			SchemaID     string                `json:"schema_id"`
			BudgetTokens int                   `json:"budget_tokens"`
			SharedWith   []domain.L1FieldShare `json:"shared_with"`
			Metadata     map[string]any        `json:"metadata"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		task, err := svc.StartTask(r.Context(), service.StartL1TaskInput{
			SessionID:    sessionID,
			RunID:        in.RunID,
			TeamID:       in.TeamID,
			AgentID:      in.AgentID,
			TaskKey:      in.TaskKey,
			TaskTitle:    in.TaskTitle,
			TaskGoal:     in.TaskGoal,
			ParentTaskID: in.ParentTaskID,
			SchemaID:     in.SchemaID,
			BudgetTokens: in.BudgetTokens,
			SharedWith:   in.SharedWith,
			Metadata:     in.Metadata,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l1.create_task", "memory_l1_task", task.ID, r.Header.Get("X-Request-Id"), task.TaskKey)
		writeJSON(w, http.StatusCreated, task)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleL1TaskItem(w http.ResponseWriter, r *http.Request, svc *service.MemoryL1Service, sessionID, taskID string) {
	if taskID == "" {
		writeErr(w, http.StatusBadRequest, errors.New("task id is required"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		view, err := svc.GetTask(r.Context(), taskID)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		if view.Task.SessionID != sessionID {
			writeErr(w, http.StatusNotFound, errors.New("task not found in session"))
			return
		}
		writeJSON(w, http.StatusOK, view)
	case http.MethodPatch:
		var in struct {
			Status       string                `json:"status"`
			BudgetTokens int                   `json:"budget_tokens"`
			SharedWith   []domain.L1FieldShare `json:"shared_with"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		if in.BudgetTokens > 0 {
			if err := svc.UpdateTaskBudget(r.Context(), taskID, in.BudgetTokens); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		if in.SharedWith != nil {
			if err := svc.UpdateTaskShared(r.Context(), taskID, in.SharedWith); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		if in.Status != "" {
			if err := svc.EndTask(r.Context(), taskID, domain.L1TaskStatus(in.Status)); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		view, err := svc.GetTask(r.Context(), taskID)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	case http.MethodDelete:
		if err := svc.EndTask(r.Context(), taskID, domain.L1TaskArchived); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l1.archive_task", "memory_l1_task", taskID, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

// handleL1TaskSubresource dispatches `tasks/{id}/<sub>...` paths.
// Supported sub-resources:
//   - fields                 (GET batch / PATCH batch-patch)
//   - fields/{path}          (GET / PUT / DELETE)
//   - fields/{path}/history  (GET)
//   - fields/{path}/rollback (POST)
//   - render-prompt          (POST)
func (h *HTTPHandler) handleL1TaskSubresource(w http.ResponseWriter, r *http.Request, svc *service.MemoryL1Service, sessionID, taskID string, parts []string) {
	if taskID == "" {
		writeErr(w, http.StatusBadRequest, errors.New("task id is required"))
		return
	}
	if len(parts) == 0 {
		writeErr(w, http.StatusNotFound, errors.New("unknown subresource"))
		return
	}
	switch parts[0] {
	case "render-prompt":
		h.handleL1RenderPrompt(w, r, svc, taskID)
	case "fields":
		h.handleL1Fields(w, r, svc, sessionID, taskID, parts[1:])
	default:
		writeErr(w, http.StatusNotFound, errors.New("unknown subresource"))
	}
}

func (h *HTTPHandler) handleL1RenderPrompt(w http.ResponseWriter, r *http.Request, svc *service.MemoryL1Service, taskID string) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	viewer := r.URL.Query().Get("viewer_agent_id")
	block, err := svc.RenderForPrompt(r.Context(), taskID, viewer)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, block)
}

func (h *HTTPHandler) handleL1Fields(w http.ResponseWriter, r *http.Request, svc *service.MemoryL1Service, sessionID, taskID string, parts []string) {
	_ = sessionID
	switch len(parts) {
	case 0:
		switch r.Method {
		case http.MethodGet:
			includeInternal := r.URL.Query().Get("include_internal") == "true"
			fields, err := svc.ListFieldsByTask(r.Context(), taskID, includeInternal)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, listResponse[domain.MemoryL1Field]{Items: fields})
		case http.MethodPatch:
			var in struct {
				Patches []domain.L1FieldPatch `json:"patches"`
			}
			if !decodeBody(w, r, &in) {
				return
			}
			results, err := svc.PatchFields(r.Context(), taskID, in.Patches)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, listResponse[domain.MemoryL1Field]{Items: results})
		default:
			methodNotAllowed(w)
		}
		return
	case 1:
		h.handleL1FieldItem(w, r, svc, taskID, parts[0])
		return
	case 2:
		fieldPath := parts[0]
		switch parts[1] {
		case "history":
			if r.Method != http.MethodGet {
				methodNotAllowed(w)
				return
			}
			limit := parsePositiveInt(r.URL.Query().Get("limit"), 20)
			items, err := svc.ListFieldHistory(r.Context(), taskID, fieldPath, limit)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, listResponse[domain.MemoryL1FieldHistory]{Items: items})
		case "rollback":
			if r.Method != http.MethodPost {
				methodNotAllowed(w)
				return
			}
			var in struct {
				ToRevision int    `json:"to_revision"`
				ChangedBy  string `json:"changed_by"`
			}
			if !decodeBody(w, r, &in) {
				return
			}
			field, err := svc.RollbackField(r.Context(), taskID, fieldPath, in.ToRevision, in.ChangedBy)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, field)
		default:
			writeErr(w, http.StatusNotFound, errors.New("unknown field action"))
		}
		return
	}
	writeErr(w, http.StatusNotFound, errors.New("unknown field path"))
}

func (h *HTTPHandler) handleL1FieldItem(w http.ResponseWriter, r *http.Request, svc *service.MemoryL1Service, taskID, fieldPath string) {
	if fieldPath == "" {
		writeErr(w, http.StatusBadRequest, errors.New("field path is required"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		field, err := svc.GetField(r.Context(), taskID, fieldPath)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, field)
	case http.MethodPut:
		var patch domain.L1FieldPatch
		if !decodeBody(w, r, &patch) {
			return
		}
		patch.FieldPath = fieldPath
		stored, err := svc.SetField(r.Context(), taskID, patch)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, stored)
	case http.MethodDelete:
		if err := svc.DeleteField(r.Context(), taskID, fieldPath); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

// handleL1Schemas serves /api/v1/memory/l1/schemas. GET filters by scope_type
// and scope_id query params; POST upserts a new schema row. The schema body
// itself stays as a JSON string so future Phase 2 validators can plug in
// without a transport-layer change.
func (h *HTTPHandler) handleL1Schemas(w http.ResponseWriter, r *http.Request) {
	svc := h.l1Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L1 service is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		scopeType := r.URL.Query().Get("scope_type")
		scopeID := r.URL.Query().Get("scope_id")
		schemas, err := svc.ListSchemas(r.Context(), scopeType, scopeID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse[domain.MemoryL1Schema]{Items: schemas})
	case http.MethodPost:
		var in domain.MemoryL1Schema
		if !decodeBody(w, r, &in) {
			return
		}
		if in.ID == "" {
			in.ID = "l1schema_" + strings.ReplaceAll(in.SchemaKey, ".", "_")
		}
		stored, err := svc.UpsertSchema(r.Context(), in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("l1.upsert_schema", "memory_l1_schema", stored.ID, r.Header.Get("X-Request-Id"), stored.SchemaKey)
		writeJSON(w, http.StatusCreated, stored)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleL1SchemaByID(w http.ResponseWriter, r *http.Request) {
	id := idFromPath(r.URL.Path, "/api/v1/memory/l1/schemas/")
	if id == "" {
		writeErr(w, http.StatusBadRequest, errors.New("schema id is required"))
		return
	}
	svc := h.l1Service()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("memory L1 service is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		stored, err := svc.GetSchema(r.Context(), id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, stored)
	case http.MethodPatch:
		stored, err := svc.GetSchema(r.Context(), id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		var in domain.MemoryL1Schema
		if !decodeBody(w, r, &in) {
			return
		}
		if in.SchemaJSON != "" {
			stored.SchemaJSON = in.SchemaJSON
		}
		if in.Description != "" {
			stored.Description = in.Description
		}
		stored.Enabled = in.Enabled
		updated, err := svc.UpsertSchema(r.Context(), stored)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := svc.DeleteSchema(r.Context(), id); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

// l1Service is a small accessor that hides the chatSvc indirection from the
// handlers above. Returning nil triggers a 503 in callers so misconfigured
// builds (no MemoryL1Service injected) don't panic.
func (h *HTTPHandler) l1Service() *service.MemoryL1Service {
	if h.chatSvc == nil {
		return nil
	}
	return h.chatSvc.MemoryL1()
}
