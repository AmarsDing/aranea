// Package service – MemoryL4Service is the L4 persistent / knowledge-graph
// façade described in `aranea/docs/16 memory-L4-persistent.md`. Phase 1
// ships entity / relation CRUD with deduplication, neighborhood traversal,
// version history, prompt rendering for L0 injection, and a stub for the
// extraction pipeline (Phase 2).
//
// The service is intentionally synchronous: extraction worker scheduling
// and embedding generation are owned by the caller (cmd/server) so tests
// can drive the methods inline. All mutating operations also write an
// audit log entry so administrators can reconstruct who changed what.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// MemoryL4Service mediates between the HTTP / L0 / L2 / L3 callers and the
// SQLite L4 repository. The L3 dependency is narrow: only fact-link
// operations need it, and they degrade gracefully when the L3 service is
// not wired in.
type MemoryL4Service struct {
	repo repository.Store
	now  func() string
}

// NewMemoryL4Service builds the service over a repository. Callers may
// later inject embedding / extraction sources once Phase 2 lands.
func NewMemoryL4Service(repo repository.Store) *MemoryL4Service {
	return &MemoryL4Service{repo: repo, now: nowUTC}
}

// SetClock overrides the clock for tests.
func (s *MemoryL4Service) SetClock(now func() string) {
	if now != nil {
		s.now = now
	}
}

// --- Inputs / outputs --------------------------------------------------------

// EntityUpsertInput is the parameter object accepted by both the HTTP
// POST/PATCH path and the extraction pipeline. NameNormalized is computed
// when empty.
type EntityUpsertInput struct {
	ID          string               `json:"id,omitempty"`
	ScopeType   domain.ScopeType     `json:"scope_type"`
	ScopeID     string               `json:"scope_id"`
	WorkspaceID string               `json:"workspace_id,omitempty"`
	UserID      string               `json:"user_id,omitempty"`
	EntityType  domain.EntityType    `json:"entity_type"`
	Name        string               `json:"name"`
	Aliases     []string             `json:"aliases,omitempty"`
	Description string               `json:"description,omitempty"`
	Attributes  map[string]any       `json:"attributes,omitempty"`
	Importance  float64              `json:"importance,omitempty"`
	Confidence  float64              `json:"confidence,omitempty"`
	SourceKind  string               `json:"source_kind,omitempty"`
	Evidence    []domain.EvidenceRef `json:"evidence,omitempty"`
	Metadata    map[string]any       `json:"metadata,omitempty"`
	By          string               `json:"by,omitempty"`
	Reason      string               `json:"reason,omitempty"`
}

// RelationUpsertInput is the parameter object accepted by both the HTTP
// POST path and the extraction pipeline.
type RelationUpsertInput struct {
	ID            string               `json:"id,omitempty"`
	ScopeType     domain.ScopeType     `json:"scope_type"`
	ScopeID       string               `json:"scope_id"`
	WorkspaceID   string               `json:"workspace_id,omitempty"`
	SourceID      string               `json:"source_id"`
	TargetID      string               `json:"target_id"`
	RelationType  domain.RelationType  `json:"relation_type"`
	Bidirectional bool                 `json:"bidirectional,omitempty"`
	Weight        float64              `json:"weight,omitempty"`
	Confidence    float64              `json:"confidence,omitempty"`
	Importance    float64              `json:"importance,omitempty"`
	Attributes    map[string]any       `json:"attributes,omitempty"`
	Evidence      []domain.EvidenceRef `json:"evidence,omitempty"`
	SourceKind    string               `json:"source_kind,omitempty"`
	By            string               `json:"by,omitempty"`
	Reason        string               `json:"reason,omitempty"`
}

// EntityListResult is the wire shape of GET §6.2 list endpoints.
type EntityListResult struct {
	Items  []domain.MemoryEntity `json:"items"`
	Total  int                   `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

// ExtractionReport summarises a single ExtractFromEpisode / Fact call.
// The Phase 1 stub always returns zeros but the shape mirrors the doc so
// the HTTP layer can be wired now.
type ExtractionReport struct {
	NewEntities      int    `json:"new_entities"`
	UpdatedEntities  int    `json:"updated_entities"`
	NewRelations     int    `json:"new_relations"`
	UpdatedRelations int    `json:"updated_relations"`
	Skipped          int    `json:"skipped"`
	Errors           int    `json:"errors"`
	Note             string `json:"note,omitempty"`
}

// --- Entity CRUD -------------------------------------------------------------

// UpsertEntity stores or updates an entity row, writing an audit log
// entry and a `memory_entity_versions` snapshot in the same transaction.
func (s *MemoryL4Service) UpsertEntity(ctx context.Context, in EntityUpsertInput) (domain.MemoryEntity, error) {
	if in.ScopeType == "" {
		return domain.MemoryEntity{}, validationError("scope_type is required")
	}
	if !in.ScopeType.IsValid() {
		return domain.MemoryEntity{}, validationError("scope_type must be one of global/workspace/user/team/agent")
	}
	if in.EntityType == "" {
		return domain.MemoryEntity{}, validationError("entity_type is required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return domain.MemoryEntity{}, validationError("name is required")
	}

	normalized := normalizeEntityName(in.Name)
	now := s.now()

	entity := domain.MemoryEntity{
		ID:             in.ID,
		ScopeType:      in.ScopeType,
		ScopeID:        in.ScopeID,
		WorkspaceID:    in.WorkspaceID,
		UserID:         in.UserID,
		EntityType:     in.EntityType,
		Name:           strings.TrimSpace(in.Name),
		NameNormalized: normalized,
		Aliases:        normalizeAliases(in.Aliases),
		Description:    strings.TrimSpace(in.Description),
		Attributes:     in.Attributes,
		Importance:     clamp01OrDefault(in.Importance, 0.5),
		Confidence:     clamp01OrDefault(in.Confidence, 0.7),
		SourceKind:     defaultIfEmpty(in.SourceKind, domain.GraphSourceUser),
		Status:         domain.EntityStatusActive,
		Metadata:       in.Metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	existing, getErr := s.repo.GetEntityByName(in.ScopeType, in.ScopeID, in.EntityType, normalized)
	isUpdate := getErr == nil
	if isUpdate {
		entity.ID = existing.ID
		entity.CreatedAt = existing.CreatedAt
		entity.UseCount = existing.UseCount
	}
	if entity.ID == "" {
		entity.ID = newID()
	}

	stored, err := s.repo.UpsertEntity(entity)
	if err != nil {
		return domain.MemoryEntity{}, err
	}

	version := 1
	if isUpdate {
		prev, _ := s.repo.ListEntityVersions(stored.ID, 1)
		if len(prev) > 0 {
			version = prev[0].Version + 1
		} else {
			version = 2
		}
	}
	snapshotJSON, _ := json.Marshal(stored)
	reason := defaultIfEmpty(in.Reason, "create")
	if isUpdate {
		reason = defaultIfEmpty(in.Reason, "update")
	}
	if vErr := s.repo.InsertEntityVersion(domain.MemoryEntityVersion{
		ID:           newID(),
		EntityID:     stored.ID,
		Version:      version,
		SnapshotJSON: string(snapshotJSON),
		ChangedBy:    in.By,
		ChangeReason: reason,
		CreatedAt:    now,
	}); vErr != nil {
		return stored, vErr
	}
	action := "memory.l4.entity.create"
	if isUpdate {
		action = "memory.l4.entity.update"
	}
	_ = s.audit(action, "memory_entities", stored.ID, map[string]any{
		"name":        stored.Name,
		"entity_type": stored.EntityType,
		"scope":       stored.ScopeType,
		"scope_id":    stored.ScopeID,
		"by":          in.By,
		"reason":      in.Reason,
	})
	return stored, nil
}

// GetEntity returns a single entity by ID.
func (s *MemoryL4Service) GetEntity(ctx context.Context, id string) (domain.MemoryEntity, error) {
	if id == "" {
		return domain.MemoryEntity{}, validationError("id is required")
	}
	return s.repo.GetEntity(id)
}

// ListEntities returns a paginated entity list scoped by the query.
func (s *MemoryL4Service) ListEntities(ctx context.Context, q repository.EntityListQuery) (EntityListResult, error) {
	items, total, err := s.repo.ListEntities(q)
	if err != nil {
		return EntityListResult{}, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	return EntityListResult{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

// ArchiveEntity flips the entity status to `archived` and records an
// audit entry. Used for soft-delete from the management UI.
func (s *MemoryL4Service) ArchiveEntity(ctx context.Context, id, by, reason string) error {
	if id == "" {
		return validationError("id is required")
	}
	now := s.now()
	if err := s.repo.UpdateEntityStatus(id, domain.EntityStatusArchived, "", now, ""); err != nil {
		return err
	}
	_ = s.audit("memory.l4.entity.archive", "memory_entities", id, map[string]any{"by": by, "reason": reason})
	return nil
}

// DeleteEntity flips the entity status to `deleted` and records an audit
// entry. Hard delete is not supported — soft delete preserves graph
// integrity for relations that referenced the entity.
func (s *MemoryL4Service) DeleteEntity(ctx context.Context, id, by, reason string) error {
	if id == "" {
		return validationError("id is required")
	}
	now := s.now()
	if err := s.repo.UpdateEntityStatus(id, domain.EntityStatusDeleted, "", "", now); err != nil {
		return err
	}
	_ = s.audit("memory.l4.entity.delete", "memory_entities", id, map[string]any{"by": by, "reason": reason})
	return nil
}

// RenameEntity updates an entity's name (and the canonical normalized
// form). A new version snapshot is written to preserve history.
func (s *MemoryL4Service) RenameEntity(ctx context.Context, id, newName, by, reason string) (domain.MemoryEntity, error) {
	if id == "" {
		return domain.MemoryEntity{}, validationError("id is required")
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return domain.MemoryEntity{}, validationError("name is required")
	}
	existing, err := s.repo.GetEntity(id)
	if err != nil {
		return domain.MemoryEntity{}, err
	}
	normalized := normalizeEntityName(newName)
	if err := s.repo.UpdateEntityName(id, newName, normalized); err != nil {
		return domain.MemoryEntity{}, err
	}
	stored, err := s.repo.GetEntity(id)
	if err != nil {
		return domain.MemoryEntity{}, err
	}
	prev, _ := s.repo.ListEntityVersions(id, 1)
	nextVersion := 1
	if len(prev) > 0 {
		nextVersion = prev[0].Version + 1
	}
	snap, _ := json.Marshal(stored)
	_ = s.repo.InsertEntityVersion(domain.MemoryEntityVersion{
		ID:           newID(),
		EntityID:     id,
		Version:      nextVersion,
		SnapshotJSON: string(snap),
		ChangedBy:    by,
		ChangeReason: defaultIfEmpty(reason, "rename"),
		DiffJSON:     fmt.Sprintf(`{"name":{"before":%q,"after":%q}}`, existing.Name, newName),
		CreatedAt:    s.now(),
	})
	_ = s.audit("memory.l4.entity.rename", "memory_entities", id, map[string]any{
		"before": existing.Name, "after": newName, "by": by, "reason": reason,
	})
	return stored, nil
}

// MergeEntities marks each `mergeIDs` row as merged into `primaryID`,
// rewires their relations to the primary entity, and writes a snapshot.
// Relations that would become self-loops are archived instead of moved.
func (s *MemoryL4Service) MergeEntities(ctx context.Context, primaryID string, mergeIDs []string, by, reason string) error {
	if primaryID == "" {
		return validationError("primary id is required")
	}
	if len(mergeIDs) == 0 {
		return validationError("merge ids are required")
	}
	primary, err := s.repo.GetEntity(primaryID)
	if err != nil {
		return err
	}
	now := s.now()
	for _, mid := range mergeIDs {
		if mid == "" || mid == primaryID {
			continue
		}
		victim, err := s.repo.GetEntity(mid)
		if err != nil {
			continue
		}
		rels, err := s.repo.ListRelationsForNode(mid, 500)
		if err != nil {
			return err
		}
		for _, rel := range rels {
			rewritten := rel
			selfLoop := false
			if rewritten.SourceID == mid {
				rewritten.SourceID = primaryID
			}
			if rewritten.TargetID == mid {
				rewritten.TargetID = primaryID
			}
			if rewritten.SourceID == rewritten.TargetID {
				selfLoop = true
			}
			if selfLoop {
				_ = s.repo.UpdateRelationStatus(rel.ID, domain.RelationStatusArchived, now, "")
				continue
			}
			rewritten.ID = newID()
			rewritten.UpdatedAt = now
			if _, err := s.repo.UpsertRelation(rewritten); err != nil {
				return err
			}
			_ = s.repo.UpdateRelationStatus(rel.ID, domain.RelationStatusArchived, now, "")
		}
		if err := s.repo.UpdateEntityStatus(mid, domain.EntityStatusMerged, primaryID, now, ""); err != nil {
			return err
		}
		snap, _ := json.Marshal(victim)
		prev, _ := s.repo.ListEntityVersions(mid, 1)
		next := 1
		if len(prev) > 0 {
			next = prev[0].Version + 1
		}
		_ = s.repo.InsertEntityVersion(domain.MemoryEntityVersion{
			ID:           newID(),
			EntityID:     mid,
			Version:      next,
			SnapshotJSON: string(snap),
			ChangedBy:    by,
			ChangeReason: defaultIfEmpty(reason, "merge"),
			DiffJSON:     fmt.Sprintf(`{"merged_into":%q}`, primaryID),
			CreatedAt:    now,
		})
		_ = s.audit("memory.l4.entity.merge", "memory_entities", mid, map[string]any{
			"merged_into": primaryID,
			"by":          by,
			"reason":      reason,
		})
	}
	_ = s.audit("memory.l4.entity.merge_target", "memory_entities", primary.ID, map[string]any{
		"sources": mergeIDs,
		"by":      by,
		"reason":  reason,
	})
	return nil
}

// ListEntityFacts returns the fact ids linked to an entity. Phase 2 wires
// the bidirectional fact ↔ entity index when L3 extraction lands.
func (s *MemoryL4Service) ListEntityFacts(ctx context.Context, entityID string, limit int) ([]domain.MemoryEntityFactLink, error) {
	if entityID == "" {
		return nil, validationError("entity id is required")
	}
	return s.repo.ListFactsForEntity(entityID, limit)
}

// ListEntityVersions returns the snapshot history of an entity newest
// first.
func (s *MemoryL4Service) ListEntityVersions(ctx context.Context, entityID string, limit int) ([]domain.MemoryEntityVersion, error) {
	if entityID == "" {
		return nil, validationError("entity id is required")
	}
	return s.repo.ListEntityVersions(entityID, limit)
}

// LinkEntityToFact upserts the entity ↔ fact reverse-index row used by
// L3 extraction (Phase 2). Exposed publicly for plugin / extraction
// pipelines.
func (s *MemoryL4Service) LinkEntityToFact(ctx context.Context, entityID, factID string, weight float64) error {
	if entityID == "" || factID == "" {
		return validationError("entity_id and fact_id are required")
	}
	return s.repo.UpsertEntityFact(entityID, factID, weight)
}

// --- Relation CRUD -----------------------------------------------------------

// UpsertRelation stores or updates a relation row keyed on
// (scope_type, scope_id, source, target, relation_type).
func (s *MemoryL4Service) UpsertRelation(ctx context.Context, in RelationUpsertInput) (domain.MemoryRelation, error) {
	if in.ScopeType == "" {
		return domain.MemoryRelation{}, validationError("scope_type is required")
	}
	if !in.ScopeType.IsValid() {
		return domain.MemoryRelation{}, validationError("scope_type must be one of global/workspace/user/team/agent")
	}
	if in.SourceID == "" || in.TargetID == "" {
		return domain.MemoryRelation{}, validationError("source_id and target_id are required")
	}
	if in.SourceID == in.TargetID {
		return domain.MemoryRelation{}, validationError("source_id and target_id must differ")
	}
	if in.RelationType == "" {
		return domain.MemoryRelation{}, validationError("relation_type is required")
	}
	now := s.now()
	relation := domain.MemoryRelation{
		ID:            in.ID,
		ScopeType:     in.ScopeType,
		ScopeID:       in.ScopeID,
		WorkspaceID:   in.WorkspaceID,
		SourceID:      in.SourceID,
		TargetID:      in.TargetID,
		RelationType:  in.RelationType,
		Bidirectional: in.Bidirectional,
		Weight:        clampPositiveOrDefault(in.Weight, 1.0),
		Confidence:    clamp01OrDefault(in.Confidence, 0.7),
		Importance:    clamp01OrDefault(in.Importance, 0.5),
		Attributes:    in.Attributes,
		Evidence:      in.Evidence,
		SourceKind:    defaultIfEmpty(in.SourceKind, domain.GraphSourceUser),
		Status:        domain.RelationStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if relation.ID == "" {
		relation.ID = newID()
	}
	stored, err := s.repo.UpsertRelation(relation)
	if err != nil {
		return domain.MemoryRelation{}, err
	}
	_ = s.audit("memory.l4.relation.upsert", "memory_relations", stored.ID, map[string]any{
		"source": stored.SourceID, "target": stored.TargetID,
		"relation_type": stored.RelationType,
		"by":            in.By, "reason": in.Reason,
	})
	return stored, nil
}

// GetRelation returns a single relation by ID.
func (s *MemoryL4Service) GetRelation(ctx context.Context, id string) (domain.MemoryRelation, error) {
	if id == "" {
		return domain.MemoryRelation{}, validationError("id is required")
	}
	return s.repo.GetRelation(id)
}

// ListRelationsForNode returns the active relations connected to a node.
func (s *MemoryL4Service) ListRelationsForNode(ctx context.Context, nodeID string, limit int) ([]domain.MemoryRelation, error) {
	if nodeID == "" {
		return nil, validationError("node id is required")
	}
	return s.repo.ListRelationsForNode(nodeID, limit)
}

// DeleteRelation flips the relation status to `deleted` and records an
// audit entry.
func (s *MemoryL4Service) DeleteRelation(ctx context.Context, id, by, reason string) error {
	if id == "" {
		return validationError("id is required")
	}
	now := s.now()
	if err := s.repo.UpdateRelationStatus(id, domain.RelationStatusDeleted, "", now); err != nil {
		return err
	}
	_ = s.audit("memory.l4.relation.delete", "memory_relations", id, map[string]any{"by": by, "reason": reason})
	return nil
}

// --- Neighborhood / search ---------------------------------------------------

// Neighborhood traverses up to `hops` hops outward from `centerID`,
// returning at most `maxNodes` distinct entities plus the relations that
// connect them. Hops are capped at 3 to keep latency bounded.
func (s *MemoryL4Service) Neighborhood(ctx context.Context, centerID string, hops, maxNodes int) (domain.GraphNeighborhood, error) {
	if centerID == "" {
		return domain.GraphNeighborhood{}, validationError("center id is required")
	}
	return s.repo.GetNeighborhood(centerID, hops, maxNodes)
}

// SearchByText is a simple keyword-based lookup over name / aliases /
// description. Vector search will replace this once embeddings are
// generated (Phase 2).
func (s *MemoryL4Service) SearchByText(ctx context.Context, scope domain.ScopeType, scopeID, query string, topK int) ([]domain.MemoryEntity, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if topK <= 0 || topK > 50 {
		topK = 10
	}
	q := repository.EntityListQuery{
		ScopeType: scope,
		ScopeID:   scopeID,
		Status:    domain.EntityStatusActive,
		Keyword:   query,
		Limit:     topK,
	}
	items, _, err := s.repo.ListEntities(q)
	return items, err
}

// --- Pipeline stubs ---------------------------------------------------------

// ExtractFromEpisode runs the Phase 2 dictionary-based extractor over
// an episode's title / goal / outcome / failure_reason fields. Each
// match becomes (or refreshes) a knowledge-graph entity in the agent's
// scope; the episode itself is *not* reverse-linked because the schema
// does not yet provide an episode↔entity index — Phase 3 will widen the
// link table. Returns Skipped=N when nothing matched.
func (s *MemoryL4Service) ExtractFromEpisode(ctx context.Context, episodeID string) (ExtractionReport, error) {
	if episodeID == "" {
		return ExtractionReport{}, validationError("episode id is required")
	}
	episode, err := s.repo.GetEpisode(episodeID)
	if err != nil {
		return ExtractionReport{}, err
	}

	text := strings.Join([]string{
		episode.Title,
		episode.Goal,
		episode.Outcome,
		episode.OutcomeSummary,
		episode.ResultPreview,
		episode.FailureReason,
	}, "\n")

	matches := scanExtractionMatches(text)
	if len(matches) == 0 {
		return ExtractionReport{Note: "no dictionary matches"}, nil
	}

	scopeType, scopeID := extractionScopeFromEpisode(episode)
	if scopeType == "" {
		return ExtractionReport{Skipped: len(matches), Note: "episode lacks a usable scope"}, nil
	}

	report := ExtractionReport{}
	for _, m := range matches {
		existing, _ := s.repo.GetEntityByName(scopeType, scopeID, m.Type, normalizeEntityName(m.Name))
		_, err := s.UpsertEntity(ctx, EntityUpsertInput{
			ScopeType:   scopeType,
			ScopeID:     scopeID,
			WorkspaceID: episode.TeamID,
			EntityType:  m.Type,
			Name:        m.Name,
			Aliases:     m.Aliases,
			Importance:  0.4,
			Confidence:  0.55,
			SourceKind:  domain.GraphSourceExtracted,
			By:          "extractor",
			Reason:      "extract_from_episode:" + episode.ID,
			Metadata: map[string]any{
				"source_episode_id": episode.ID,
			},
		})
		if err != nil {
			report.Errors++
			continue
		}
		if existing.ID == "" {
			report.NewEntities++
		} else {
			report.UpdatedEntities++
		}
	}
	return report, nil
}

// ExtractFromFact runs the Phase 2 dictionary-based extractor over a
// fact's statement + details_markdown. Each match becomes (or refreshes)
// a knowledge-graph entity in the fact's scope and is reverse-linked
// through `memory_entity_facts` so future neighborhood queries can
// surface the originating fact alongside the entity.
func (s *MemoryL4Service) ExtractFromFact(ctx context.Context, factID string) (ExtractionReport, error) {
	if factID == "" {
		return ExtractionReport{}, validationError("fact id is required")
	}
	fact, err := s.repo.GetFact(factID)
	if err != nil {
		return ExtractionReport{}, err
	}

	text := strings.TrimSpace(fact.Statement)
	if fact.DetailsMarkdown != "" {
		text = text + "\n" + fact.DetailsMarkdown
	}

	matches := scanExtractionMatches(text)
	if len(matches) == 0 {
		return ExtractionReport{Note: "no dictionary matches"}, nil
	}

	report := ExtractionReport{}
	for _, m := range matches {
		existing, _ := s.repo.GetEntityByName(fact.ScopeType, fact.ScopeID, m.Type, normalizeEntityName(m.Name))
		entity, err := s.UpsertEntity(ctx, EntityUpsertInput{
			ScopeType:   fact.ScopeType,
			ScopeID:     fact.ScopeID,
			WorkspaceID: fact.WorkspaceID,
			UserID:      fact.UserID,
			EntityType:  m.Type,
			Name:        m.Name,
			Aliases:     m.Aliases,
			Importance:  0.45,
			Confidence:  0.6,
			SourceKind:  domain.GraphSourceExtracted,
			By:          "extractor",
			Reason:      "extract_from_fact:" + fact.ID,
			Metadata: map[string]any{
				"source_fact_id": fact.ID,
			},
		})
		if err != nil {
			report.Errors++
			continue
		}
		if existing.ID == "" {
			report.NewEntities++
		} else {
			report.UpdatedEntities++
		}
		if err := s.repo.UpsertEntityFact(entity.ID, fact.ID, 1.0); err != nil {
			report.Errors++
		}
	}
	return report, nil
}

// extractionScopeFromEpisode picks the most specific scope an episode
// can attach to. Agent-owned episodes attach to the agent's scope so
// extraction stays per-persona; team-owned episodes attach to the team
// scope; everything else is skipped because there is no obvious owner.
func extractionScopeFromEpisode(episode domain.MemoryEpisode) (domain.ScopeType, string) {
	if episode.AgentID != "" {
		return domain.ScopeAgent, episode.AgentID
	}
	if episode.TeamID != "" {
		return domain.ScopeTeam, episode.TeamID
	}
	return "", ""
}

// --- L0 prompt rendering -----------------------------------------------------

// l4MaxNeighborChars caps how much text the rendered neighborhood block
// can occupy in the L0 prompt. Aligned with §5.8 prompt-budget defaults.
const l4MaxNeighborChars = 1500

// RenderForPrompt formats a GraphNeighborhood into a markdown block
// suitable for L0 injection. Returns ok=false when the neighborhood
// holds no useful data (e.g. an isolated node).
func (s *MemoryL4Service) RenderForPrompt(n domain.GraphNeighborhood, maxChars int) (string, bool) {
	if maxChars <= 0 || maxChars > 4000 {
		maxChars = l4MaxNeighborChars
	}
	if n.Center.ID == "" || (len(n.Entities) == 0 && len(n.Relations) == 0) {
		return "", false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# memory.l4.graph\nCenter: %s (%s)", n.Center.Name, n.Center.EntityType)
	if desc := strings.TrimSpace(n.Center.Description); desc != "" {
		fmt.Fprintf(&b, "\n  - %s", desc)
	}

	entityIndex := map[string]string{n.Center.ID: n.Center.Name}
	for _, e := range n.Entities {
		entityIndex[e.ID] = e.Name
	}
	rels := append([]domain.MemoryRelation(nil), n.Relations...)
	sort.SliceStable(rels, func(i, j int) bool {
		return rels[i].Weight > rels[j].Weight
	})
	if len(rels) > 0 {
		b.WriteString("\nRelations:")
		for _, rel := range rels {
			src := entityIndex[rel.SourceID]
			if src == "" {
				src = rel.SourceID
			}
			dst := entityIndex[rel.TargetID]
			if dst == "" {
				dst = rel.TargetID
			}
			fmt.Fprintf(&b, "\n  - %s --[%s]--> %s (w=%.2f)", src, rel.RelationType, dst, rel.Weight)
			if b.Len() > maxChars {
				break
			}
		}
	}
	body := strings.TrimSpace(b.String())
	if body == "" {
		return "", false
	}
	if len(body) > maxChars {
		body = body[:maxChars] + "..."
	}
	return body, true
}

// NeighborhoodSegmentForL0 is the L0RecallSource shim: given a session /
// agent / query, it picks a center entity by keyword search across the
// scopes the agent can see (agent → workspace → user → global), expands
// to its k-hop neighborhood, and renders a `memory.l4.graph` segment.
//
// All limits come from `agent_runtime_settings` (§3.3): the segment is
// gated by `l4_enabled` AND `l4_graph_inject_neighbors`, neighbor count
// is capped at `l4_graph_max_neighbors`, and traversal hops at
// `l4_graph_max_hops` (clamped to ≤3 to keep latency bounded).
//
// Returns ok=false when the feature is disabled, the query is empty,
// no candidate entity matches, or the neighborhood renders to an empty
// block — so L0 simply omits the segment.
func (s *MemoryL4Service) NeighborhoodSegmentForL0(ctx context.Context, sessionID, agentID, query string) (domain.L0Segment, bool) {
	return s.NeighborhoodSegmentForL0WithContext(ctx, domain.L0MemoryScopeContext{
		SessionID: sessionID,
		AgentID:   agentID,
		Query:     query,
	})
}

// NeighborhoodSegmentForL0WithContext is the context-rich L0 seam. Team,
// user, and workspace scope IDs are used when choosing the center entity.
func (s *MemoryL4Service) NeighborhoodSegmentForL0WithContext(ctx context.Context, scope domain.L0MemoryScopeContext) (domain.L0Segment, bool) {
	if strings.TrimSpace(scope.Query) == "" || scope.AgentID == "" {
		return domain.L0Segment{}, false
	}
	settings, _ := s.repo.GetAgentRuntimeSettings(scope.AgentID)
	if !settings.L4Enabled || !settings.L4GraphInjectNeighbors {
		return domain.L0Segment{}, false
	}

	hops := firstPositive(settings.L4GraphMaxHops, 1)
	if hops > 3 {
		hops = 3
	}
	maxNodes := firstPositive(settings.L4GraphMaxNeighbors, 6)
	if maxNodes > 20 {
		maxNodes = 20
	}

	center, ok := s.findCenterEntity(scope, scope.Query)
	if !ok {
		return domain.L0Segment{}, false
	}

	n, err := s.repo.GetNeighborhood(center.ID, hops, maxNodes)
	if err != nil {
		return domain.L0Segment{}, false
	}
	if n.Center.ID == "" {
		n.Center = center
	}
	body, ok := s.RenderForPrompt(n, l4MaxNeighborChars)
	if !ok {
		return domain.L0Segment{}, false
	}

	_ = s.repo.BumpEntityUseCount(center.ID, s.now())

	return domain.L0Segment{
		Section: "memory.l4.graph",
		Role:    "system",
		Source:  fmt.Sprintf("memory.l4:%s", center.ID),
		Tokens:  estimateTokensApprox(body),
		Content: body,
		Preview: previewText(body, l0PreviewLimit),
	}, true
}

// findCenterEntity walks the scope hierarchy an agent can see (agent →
// workspace → user → global) and returns the first active entity whose
// name / aliases / description match the query. Phase 2's attention
// pipeline will replace this keyword search with vector recall.
func (s *MemoryL4Service) findCenterEntity(scope domain.L0MemoryScopeContext, query string) (domain.MemoryEntity, bool) {
	candidates := []repository.EntityListQuery{
		{ScopeType: domain.ScopeAgent, ScopeID: scope.AgentID},
		{ScopeType: domain.ScopeTeam, ScopeID: scope.TeamID},
		{ScopeType: domain.ScopeWorkspace, ScopeID: scope.WorkspaceID},
		{ScopeType: domain.ScopeUser, ScopeID: scope.UserID},
		{ScopeType: domain.ScopeGlobal},
	}
	for _, q := range candidates {
		if q.ScopeType != domain.ScopeGlobal && q.ScopeID == "" {
			continue
		}
		q.Status = domain.EntityStatusActive
		q.Keyword = query
		q.Limit = 1
		items, _, err := s.repo.ListEntities(q)
		if err != nil || len(items) == 0 {
			continue
		}
		return items[0], true
	}
	return domain.MemoryEntity{}, false
}

// --- Audit helper -----------------------------------------------------------

func (s *MemoryL4Service) audit(action, resource, resourceID string, detail map[string]any) error {
	body, _ := json.Marshal(detail)
	if len(body) == 0 {
		body = []byte("{}")
	}
	return s.repo.AddAuditLog(domain.AuditLog{
		ID:         newID(),
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Detail:     string(body),
	})
}

// --- Pure helpers -----------------------------------------------------------

func normalizeEntityName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}

func normalizeAliases(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return out
}

func clamp01OrDefault(v, def float64) float64 {
	if v <= 0 {
		return def
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampPositiveOrDefault(v, def float64) float64 {
	if v <= 0 {
		return def
	}
	return v
}

func defaultIfEmpty(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
