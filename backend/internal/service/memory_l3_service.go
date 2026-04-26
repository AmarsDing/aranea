// Package service – MemoryL3Service is the L3 semantic-memory façade
// described in `aranea/docs/15 memory-L3-semantic.md`. Phase 1 ships
// fact CRUD, fingerprint-based dedup, version history, BM25 recall
// (vector path is wired in but inert until embeddings are produced),
// feedback-driven confidence, conflict tracking, decay batches, and
// prompt rendering for L0 injection.
//
// The service intentionally has no goroutines of its own — decay /
// embedding worker scheduling is the caller's responsibility. Tests can
// drive RunDecayBatch / BuildEmbedding inline.
package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// MemoryL3Service mediates between the HTTP / L0 / consolidation layers
// and the SQLite repository. It is the single owner of the upsert /
// dedup / conflict-detection rules so callers can stay simple.
type MemoryL3Service struct {
	repo         repository.Store
	pii          *PIIFilter
	embedder     EmbeddingSource
	memoryL4     L4FactExtractionSource
	now          func() string
	scopeWeights map[domain.ScopeType]float64
}

// EmbeddingSource is the narrow contract MemoryL3Service uses to ask the
// LLM provider for a vector embedding of a given text. Implementations
// can wrap ProviderService or any HTTP client. The seam keeps the
// service usable in tests where embeddings are stubbed.
type EmbeddingSource interface {
	Embed(ctx context.Context, model, text string) ([]float32, error)
}

// L4FactExtractionSource is the narrow dependency used to keep the L3 -> L4
// extraction hook best-effort and acyclic.
type L4FactExtractionSource interface {
	ExtractFromFact(ctx context.Context, factID string) (ExtractionReport, error)
}

// FactListResult is the wire shape of GET §6.2 list endpoints.
type FactListResult struct {
	Items  []domain.MemoryFact `json:"items"`
	Total  int                 `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

// FactPatch is the partial update payload (§6.2 PATCH). Pointers signal
// "field present"; nil leaves the existing value untouched.
type FactPatch struct {
	Statement       *string          `json:"statement,omitempty"`
	DetailsMarkdown *string          `json:"details_markdown,omitempty"`
	Tags            *[]string        `json:"tags,omitempty"`
	Kind            *domain.FactKind `json:"fact_kind,omitempty"`
	Confidence      *float64         `json:"confidence,omitempty"`
	Importance      *float64         `json:"importance,omitempty"`
	Status          *string          `json:"status,omitempty"`
	TTLDays         *int             `json:"ttl_days,omitempty"`
	By              string           `json:"by,omitempty"`
	Reason          string           `json:"reason,omitempty"`
}

// BulkUpsertReport summarises a §5.6 BulkUpsert call.
type BulkUpsertReport struct {
	Created    int `json:"created"`
	Updated    int `json:"updated"`
	Duplicated int `json:"duplicated"`
	Errors     int `json:"errors"`
	Conflicts  int `json:"conflicts"`
}

// DecayReport summarises a §5.5 RunDecayBatch call.
type DecayReport struct {
	Processed      int     `json:"processed"`
	Archived       int     `json:"archived"`
	ConfidenceDrop float64 `json:"confidence_drop"`
}

// L3StatsReport is what GET /admin/memory/l3/stats returns.
type L3StatsReport struct {
	StatusCounts map[string]int `json:"status_counts"`
}

// NewMemoryL3Service builds a service over a repository with sensible
// defaults: regex PII filter, no embedder (vector path inert), and the
// scope weights from §5.3.
func NewMemoryL3Service(repo repository.Store) *MemoryL3Service {
	return &MemoryL3Service{
		repo: repo,
		pii:  NewPIIFilter(),
		now:  nowUTC,
		scopeWeights: map[domain.ScopeType]float64{
			domain.ScopeAgent:     1.0,
			domain.ScopeUser:      0.95,
			domain.ScopeTeam:      0.9,
			domain.ScopeWorkspace: 0.85,
			domain.ScopeGlobal:    0.8,
		},
	}
}

// SetEmbeddingSource wires the embedding provider used by Recall and
// BuildEmbedding. Nil disables the vector path (BM25 still works).
func (s *MemoryL3Service) SetEmbeddingSource(src EmbeddingSource) { s.embedder = src }

// SetL4ExtractionSource wires L4 dictionary/entity extraction after L3 fact
// writes. Extraction failures are audited but never block the fact write.
func (s *MemoryL3Service) SetL4ExtractionSource(src L4FactExtractionSource) { s.memoryL4 = src }

// SetClock overrides the clock for tests.
func (s *MemoryL3Service) SetClock(now func() string) {
	if now != nil {
		s.now = now
	}
}

// --- Write paths ------------------------------------------------------------

// UpsertFact applies the §5.2 algorithm: PII detection, normalisation,
// fingerprint dedup, version snapshot, audit log. Returns the resulting
// fact (newly-created or updated).
func (s *MemoryL3Service) UpsertFact(ctx context.Context, in domain.FactUpsertInput) (domain.MemoryFact, error) {
	_ = ctx
	if !in.ScopeType.IsValid() {
		return domain.MemoryFact{}, validationError("scope_type %q is invalid", in.ScopeType)
	}
	if strings.TrimSpace(in.Statement) == "" {
		return domain.MemoryFact{}, validationError("statement is required")
	}
	statement := strings.TrimSpace(in.Statement)
	if max := 4000; len(statement) > max {
		statement = statement[:max]
	}
	details := strings.TrimSpace(in.DetailsMarkdown)
	if max := 4000; len(details) > max {
		details = details[:max]
	}
	if in.Kind == "" {
		in.Kind = domain.FactGeneric
	}
	if !in.Kind.IsValid() {
		return domain.MemoryFact{}, validationError("fact_kind %q is invalid", in.Kind)
	}

	scope := in.ScopeType
	scopeID := strings.TrimSpace(in.ScopeID)
	if scopeID == "" {
		scopeID = inferScopeID(scope, in)
	}

	piiHit, redacted := s.pii.RedactPII(statement)
	piiHitDetails, redactedDetails := s.pii.RedactPII(details)
	if piiHit || piiHitDetails {
		// Spec §5.2 step 2: when PII is detected, force the scope to
		// user (or agent) so the redacted form is never shared. We pick
		// "user" because most PII is user-scoped; if the upstream knows
		// better it can pre-set the scope correctly.
		if scope != domain.ScopeAgent && scope != domain.ScopeUser {
			scope = domain.ScopeUser
			if scopeID == "" {
				scopeID = strings.TrimSpace(in.UserID)
			}
		}
	}

	normalized := normalizeStatement(statement)
	fp := fingerprintForStatement(scope, scopeID, normalized)

	confidence := clampUnit(in.Confidence)
	if confidence == 0 {
		confidence = 0.7
	}
	importance := clampUnit(in.Importance)
	if importance == 0 {
		importance = 0.5
	}

	tags := dedupStrings(in.Tags)
	metaJSON := encodeMetaJSON(in.Metadata)

	existing, err := s.repo.GetFactByFingerprint(scope, scopeID, fp)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.MemoryFact{}, err
	}
	if err == nil && existing.ID != "" {
		// Update path — bump confidence (capped) and merge tags.
		existing.Statement = statement
		existing.StatementNormalized = normalized
		existing.DetailsMarkdown = chooseNonEmpty(details, existing.DetailsMarkdown)
		existing.Kind = in.Kind
		existing.Tags = mergeStringLists(existing.Tags, tags)
		existing.Confidence = clampUnit(existing.Confidence + 0.05)
		existing.Importance = math.Max(existing.Importance, importance)
		existing.Version++
		if in.SourceKind != "" {
			existing.SourceKind = in.SourceKind
		}
		if in.SourceEpisodeID != "" {
			existing.SourceEpisodeID = in.SourceEpisodeID
		}
		if in.SourceSessionID != "" {
			existing.SourceSessionID = in.SourceSessionID
		}
		if in.SourceMessageID != "" {
			existing.SourceMessageID = in.SourceMessageID
		}
		if in.SourceExternal != "" {
			existing.SourceExternal = in.SourceExternal
		}
		if in.TTLDays > 0 {
			existing.TTLDays = in.TTLDays
		}
		if metaJSON != "{}" {
			existing.MetadataJSON = metaJSON
		}
		existing.PIIFlag = existing.PIIFlag || piiHit || piiHitDetails
		if piiHit {
			existing.RedactedStatement = redacted
		}
		if piiHitDetails {
			existing.DetailsMarkdown = redactedDetails
		}
		if err = s.repo.UpdateFact(existing); err != nil {
			return domain.MemoryFact{}, err
		}
		_ = s.recordVersion(existing, "update", in.By)
		_ = s.refreshFTS(existing)
		_ = s.audit("memory.l3.upsert", "memory_facts", existing.ID, map[string]any{
			"scope":  string(existing.ScopeType),
			"reason": "fingerprint_match",
			"by":     in.By,
		})
		updated, err := s.repo.GetFact(existing.ID)
		if err != nil {
			return domain.MemoryFact{}, err
		}
		s.extractFactToL4(ctx, updated.ID)
		return updated, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.MemoryFact{}, err
	}

	// Create path.
	fact := domain.MemoryFact{
		ID:                  newID(),
		ScopeType:           scope,
		ScopeID:             scopeID,
		WorkspaceID:         in.WorkspaceID,
		UserID:              in.UserID,
		TeamID:              in.TeamID,
		AgentID:             in.AgentID,
		Statement:           statement,
		StatementNormalized: normalized,
		Fingerprint:         fp,
		DetailsMarkdown:     details,
		Kind:                in.Kind,
		Tags:                tags,
		Confidence:          confidence,
		Importance:          importance,
		SourceKind:          chooseNonEmpty(in.SourceKind, "user"),
		SourceEpisodeID:     in.SourceEpisodeID,
		SourceSessionID:     in.SourceSessionID,
		SourceMessageID:     in.SourceMessageID,
		SourceExternal:      in.SourceExternal,
		Version:             1,
		Status:              domain.FactStatusActive,
		EmbeddingStatus:     "pending",
		PIIFlag:             piiHit || piiHitDetails,
		RedactedStatement:   redacted,
		TTLDays:             in.TTLDays,
		DecayFactor:         0.98,
		MetadataJSON:        metaJSON,
		LastUsedAt:          s.now(),
	}
	if piiHitDetails {
		fact.DetailsMarkdown = redactedDetails
	}
	created, err := s.repo.CreateFact(fact)
	if err != nil {
		return domain.MemoryFact{}, err
	}
	_ = s.recordVersion(created, "create", in.By)
	_ = s.refreshFTS(created)
	_ = s.audit("memory.l3.upsert", "memory_facts", created.ID, map[string]any{
		"scope":  string(created.ScopeType),
		"reason": "create",
		"by":     in.By,
	})
	s.extractFactToL4(ctx, created.ID)
	return created, nil
}

// BulkUpsert iterates UpsertFact and aggregates the per-row outcome.
// Errors don't abort the batch; they are recorded in the report instead.
func (s *MemoryL3Service) BulkUpsert(ctx context.Context, ins []domain.FactUpsertInput) (BulkUpsertReport, error) {
	out := BulkUpsertReport{}
	for _, in := range ins {
		before, _ := s.repo.GetFactByFingerprint(in.ScopeType, in.ScopeID, fingerprintForStatement(in.ScopeType, in.ScopeID, normalizeStatement(in.Statement)))
		fact, err := s.UpsertFact(ctx, in)
		if err != nil {
			out.Errors++
			continue
		}
		if before.ID == "" || before.ID != fact.ID {
			out.Created++
		} else {
			out.Updated++
		}
	}
	return out, nil
}

// UpdateFact applies the partial patch and writes a version snapshot.
func (s *MemoryL3Service) UpdateFact(ctx context.Context, id string, patch FactPatch) (domain.MemoryFact, error) {
	_ = ctx
	if id == "" {
		return domain.MemoryFact{}, validationError("id is required")
	}
	fact, err := s.repo.GetFact(id)
	if err != nil {
		return domain.MemoryFact{}, err
	}
	changed := false
	if patch.Statement != nil && strings.TrimSpace(*patch.Statement) != "" {
		stmt := strings.TrimSpace(*patch.Statement)
		piiHit, redacted := s.pii.RedactPII(stmt)
		fact.Statement = stmt
		fact.StatementNormalized = normalizeStatement(stmt)
		fact.Fingerprint = fingerprintForStatement(fact.ScopeType, fact.ScopeID, fact.StatementNormalized)
		fact.PIIFlag = fact.PIIFlag || piiHit
		if piiHit {
			fact.RedactedStatement = redacted
		}
		changed = true
	}
	if patch.DetailsMarkdown != nil {
		details := strings.TrimSpace(*patch.DetailsMarkdown)
		piiHit, redacted := s.pii.RedactPII(details)
		if piiHit {
			details = redacted
			fact.PIIFlag = true
		}
		fact.DetailsMarkdown = details
		changed = true
	}
	if patch.Tags != nil {
		fact.Tags = dedupStrings(*patch.Tags)
		changed = true
	}
	if patch.Kind != nil && (*patch.Kind).IsValid() {
		fact.Kind = *patch.Kind
		changed = true
	}
	if patch.Confidence != nil {
		fact.Confidence = clampUnit(*patch.Confidence)
		changed = true
	}
	if patch.Importance != nil {
		fact.Importance = clampUnit(*patch.Importance)
		changed = true
	}
	if patch.Status != nil && strings.TrimSpace(*patch.Status) != "" {
		fact.Status = strings.TrimSpace(*patch.Status)
		changed = true
	}
	if patch.TTLDays != nil {
		fact.TTLDays = *patch.TTLDays
		changed = true
	}
	if !changed {
		return fact, nil
	}
	fact.Version++
	if err = s.repo.UpdateFact(fact); err != nil {
		return domain.MemoryFact{}, err
	}
	_ = s.recordVersion(fact, chooseNonEmpty(patch.Reason, "update"), patch.By)
	_ = s.refreshFTS(fact)
	_ = s.audit("memory.l3.update", "memory_facts", fact.ID, map[string]any{
		"by":     patch.By,
		"reason": patch.Reason,
	})
	updated, err := s.repo.GetFact(fact.ID)
	if err != nil {
		return domain.MemoryFact{}, err
	}
	s.extractFactToL4(ctx, updated.ID)
	return updated, nil
}

// DeleteFact soft-deletes a fact and removes it from the indexes.
func (s *MemoryL3Service) DeleteFact(ctx context.Context, id, by string) error {
	_ = ctx
	if id == "" {
		return validationError("id is required")
	}
	now := s.now()
	if err := s.repo.UpdateFactStatus(id, domain.FactStatusDeleted, "", now); err != nil {
		return err
	}
	_ = s.repo.DeleteFactIndex(id)
	_ = s.audit("memory.l3.delete", "memory_facts", id, map[string]any{"by": by})
	return nil
}

// RollbackFact restores a previous version. It writes a new version row
// (so the rollback itself is auditable) and bumps the live version
// number rather than overwriting it.
func (s *MemoryL3Service) RollbackFact(ctx context.Context, id string, toVersion int, by string) (domain.MemoryFact, error) {
	_ = ctx
	if id == "" || toVersion <= 0 {
		return domain.MemoryFact{}, validationError("id and to_version are required")
	}
	fact, err := s.repo.GetFact(id)
	if err != nil {
		return domain.MemoryFact{}, err
	}
	target, err := s.repo.GetFactVersion(id, toVersion)
	if err != nil {
		return domain.MemoryFact{}, err
	}
	fact.Statement = target.Statement
	fact.StatementNormalized = normalizeStatement(target.Statement)
	fact.Fingerprint = fingerprintForStatement(fact.ScopeType, fact.ScopeID, fact.StatementNormalized)
	fact.DetailsMarkdown = target.Details
	fact.Tags = target.Tags
	fact.Confidence = target.Confidence
	fact.Status = chooseNonEmpty(target.Status, fact.Status)
	fact.Version++
	if err = s.repo.UpdateFact(fact); err != nil {
		return domain.MemoryFact{}, err
	}
	_ = s.recordVersion(fact, fmt.Sprintf("rollback_to_v%d", toVersion), by)
	_ = s.refreshFTS(fact)
	_ = s.audit("memory.l3.rollback", "memory_facts", id, map[string]any{"to": toVersion, "by": by})
	return s.repo.GetFact(id)
}

// --- Read paths -------------------------------------------------------------

// Get returns a single fact by ID.
func (s *MemoryL3Service) Get(ctx context.Context, id string) (domain.MemoryFact, error) {
	_ = ctx
	if id == "" {
		return domain.MemoryFact{}, validationError("id is required")
	}
	return s.repo.GetFact(id)
}

// List returns paginated facts using the repository filter struct.
func (s *MemoryL3Service) List(ctx context.Context, q repository.FactListQuery) (FactListResult, error) {
	_ = ctx
	items, total, err := s.repo.ListFacts(q)
	if err != nil {
		return FactListResult{}, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	return FactListResult{Items: items, Total: total, Limit: limit, Offset: q.Offset}, nil
}

// ListVersions returns the history rows for a fact (latest first).
func (s *MemoryL3Service) ListVersions(ctx context.Context, factID string, limit int) ([]domain.FactVersion, error) {
	_ = ctx
	return s.repo.ListFactVersions(factID, limit)
}

// ListFeedback returns the most-recent feedback entries.
func (s *MemoryL3Service) ListFeedback(ctx context.Context, factID string, limit int) ([]domain.FactFeedback, error) {
	_ = ctx
	return s.repo.ListFactFeedback(factID, limit)
}

// Recall implements the §5.3 hybrid retrieval. Vector + BM25 results are
// merged by fact id; the final score combines vector / BM25 / confidence
// / recency / scope_weight using the spec coefficients.
func (s *MemoryL3Service) Recall(ctx context.Context, q domain.FactRecallQuery) ([]domain.FactRecallHit, error) {
	if q.TopK <= 0 {
		q.TopK = 5
	}
	if q.TopK > 50 {
		q.TopK = 50
	}
	if q.MinScore <= 0 {
		q.MinScore = 0.3
	}
	includes := q.IncludeScopes
	if len(includes) == 0 {
		includes = []domain.ScopeType{domain.ScopeAgent, domain.ScopeUser, domain.ScopeTeam, domain.ScopeWorkspace}
	}
	scopes, scopeIDs := s.expandScopes(includes, q)
	if len(scopes) == 0 {
		return nil, nil
	}

	queryText := strings.TrimSpace(q.Query)
	wantVector := len(q.QueryEmbedding) > 0
	if !wantVector && queryText != "" && s.embedder != nil {
		// Best-effort: ask the embedder; failure is silent so BM25 still runs.
		if vec, err := s.embedder.Embed(ctx, "", queryText); err == nil {
			q.QueryEmbedding = vec
			wantVector = true
		}
	}

	var bm25Hits, vectorHits []domain.FactRecallHit
	if queryText != "" {
		var err error
		bm25Hits, err = s.repo.SearchFactsBM25(scopes, scopeIDs, queryText, q.TopK*4)
		if err != nil {
			return nil, err
		}
	}
	if wantVector {
		var err error
		vectorHits, err = s.repo.SearchFactsVector(scopes, scopeIDs, q.QueryEmbedding, q.TopK*4)
		if err != nil {
			return nil, err
		}
	}

	merged := mergeRecallHits(bm25Hits, vectorHits)
	merged = applyRecallFilters(merged, q)
	scoreHits(merged, s.scopeWeights, s.now())
	sort.Slice(merged, func(i, j int) bool { return merged[i].FinalScore > merged[j].FinalScore })

	out := make([]domain.FactRecallHit, 0, q.TopK)
	for _, h := range merged {
		if h.FinalScore < q.MinScore {
			continue
		}
		out = append(out, h)
		if len(out) >= q.TopK {
			break
		}
	}
	for _, h := range out {
		_ = s.repo.BumpFactUseStat(h.Fact.ID, true, s.now())
	}
	return out, nil
}

// Feedback applies the §5.4 algorithm: insert the row, mutate confidence,
// auto-archive when the threshold is crossed, and auto-create a conflict
// after three consecutive rejects.
func (s *MemoryL3Service) Feedback(ctx context.Context, fb domain.FactFeedback) error {
	_ = ctx
	if fb.FactID == "" {
		return validationError("fact_id is required")
	}
	if fb.Type == "" {
		return validationError("type is required")
	}
	fact, err := s.repo.GetFact(fb.FactID)
	if err != nil {
		return err
	}
	if fb.ID == "" {
		fb.ID = newID()
	}
	if fb.Weight == 0 {
		fb.Weight = 1.0
	}
	if fb.CreatedAt == "" {
		fb.CreatedAt = s.now()
	}
	if _, err = s.repo.InsertFactFeedback(fb); err != nil {
		return err
	}
	delta := 0.0
	posInc, negInc := 0, 0
	switch fb.Type {
	case domain.FactFeedbackConfirm:
		delta = 0.10 * fb.Weight
		posInc = 1
	case domain.FactFeedbackReject:
		delta = -0.20 * fb.Weight
		negInc = 1
	case domain.FactFeedbackUsed:
		delta = 0.02 * fb.Weight
	case domain.FactFeedbackNotUsed:
		delta = -0.01 * fb.Weight
	case domain.FactFeedbackRefine:
		// refine: keep confidence, bump importance only.
		_ = s.repo.UpdateFact(domain.MemoryFact{
			ID: fact.ID, Statement: fact.Statement, StatementNormalized: fact.StatementNormalized,
			Fingerprint: fact.Fingerprint, DetailsMarkdown: fact.DetailsMarkdown, Kind: fact.Kind,
			Tags: fact.Tags, Confidence: fact.Confidence, Importance: clampUnit(fact.Importance + 0.05),
			SourceKind: fact.SourceKind, SourceEpisodeID: fact.SourceEpisodeID, SourceSessionID: fact.SourceSessionID,
			SourceMessageID: fact.SourceMessageID, SourceExternal: fact.SourceExternal,
			Version: fact.Version, Status: fact.Status, SupersededBy: fact.SupersededBy,
			PIIFlag: fact.PIIFlag, RedactedStatement: fact.RedactedStatement,
			TTLDays: fact.TTLDays, DecayFactor: fact.DecayFactor,
			NextDecayAt: fact.NextDecayAt, LastUsedAt: fact.LastUsedAt, ExpiresAt: fact.ExpiresAt,
			MetadataJSON: fact.MetadataJSON, ArchivedAt: fact.ArchivedAt, DeletedAt: fact.DeletedAt,
		})
	}
	if delta != 0 || posInc != 0 || negInc != 0 {
		newConf := clampUnit(fact.Confidence + delta)
		if err = s.repo.UpdateFactConfidence(fact.ID, newConf, 0, posInc, negInc); err != nil {
			return err
		}
		// archive when below threshold
		threshold := 0.2
		settings, sErr := s.repo.GetAgentRuntimeSettings(fb.AgentID)
		if sErr == nil && settings.L3ArchiveThreshold > 0 {
			threshold = settings.L3ArchiveThreshold
		}
		if newConf < threshold {
			_ = s.repo.UpdateFactStatus(fact.ID, domain.FactStatusArchived, "", s.now())
		}
	}
	if fb.Type == domain.FactFeedbackReject {
		recent, _ := s.repo.CountRecentFactFeedback(fact.ID, domain.FactFeedbackReject, 3)
		if recent >= 3 {
			_ = s.markSelfConflict(fact)
		}
	}
	_ = s.audit("memory.l3.feedback", "memory_facts", fact.ID, map[string]any{
		"type":   fb.Type,
		"source": fb.Source,
		"weight": fb.Weight,
	})
	return nil
}

// DetectConflicts compares the fact against high-similarity neighbours
// in the same scope. Phase 1 implementation uses BM25 as the proxy: any
// match scoring above the floor that has a different fingerprint is
// flagged as a candidate conflict for human review.
func (s *MemoryL3Service) DetectConflicts(ctx context.Context, factID string) ([]domain.FactConflict, error) {
	_ = ctx
	if factID == "" {
		return nil, validationError("fact_id is required")
	}
	fact, err := s.repo.GetFact(factID)
	if err != nil {
		return nil, err
	}
	hits, err := s.repo.SearchFactsBM25(
		[]domain.ScopeType{fact.ScopeType},
		[]string{fact.ScopeID},
		fact.Statement,
		10,
	)
	if err != nil {
		return nil, err
	}
	var out []domain.FactConflict
	for _, h := range hits {
		if h.Fact.ID == fact.ID || h.Fact.Fingerprint == fact.Fingerprint {
			continue
		}
		c := domain.FactConflict{
			FactAID:    fact.ID,
			FactBID:    h.Fact.ID,
			ScopeType:  fact.ScopeType,
			ScopeID:    fact.ScopeID,
			Kind:       domain.FactConflictOverlap,
			Similarity: h.BM25Score,
			Status:     domain.FactConflictStatusOpen,
			DetectedBy: "runtime",
		}
		saved, err := s.repo.UpsertFactConflict(c)
		if err != nil {
			continue
		}
		out = append(out, saved)
	}
	return out, nil
}

// ResolveConflict marks a conflict as resolved with the chosen action.
// When resolution = keep_a / keep_b the loser is archived automatically.
func (s *MemoryL3Service) ResolveConflict(ctx context.Context, conflictID, resolution, by string) error {
	_ = ctx
	if conflictID == "" {
		return validationError("conflict_id is required")
	}
	c, err := s.repo.GetFactConflict(conflictID)
	if err != nil {
		return err
	}
	if err = s.repo.UpdateFactConflictResolution(conflictID, domain.FactConflictStatusResolved, resolution, by, s.now()); err != nil {
		return err
	}
	switch resolution {
	case "keep_a":
		_ = s.repo.UpdateFactStatus(c.FactBID, domain.FactStatusArchived, c.FactAID, s.now())
	case "keep_b":
		_ = s.repo.UpdateFactStatus(c.FactAID, domain.FactStatusArchived, c.FactBID, s.now())
	case "mark_disputed":
		_ = s.repo.UpdateFactStatus(c.FactAID, domain.FactStatusDisputed, "", s.now())
		_ = s.repo.UpdateFactStatus(c.FactBID, domain.FactStatusDisputed, "", s.now())
	}
	_ = s.audit("memory.l3.conflict.resolve", "memory_fact_conflicts", conflictID, map[string]any{
		"resolution": resolution,
		"by":         by,
	})
	return nil
}

// ListOpenConflicts proxies the repo call.
func (s *MemoryL3Service) ListOpenConflicts(ctx context.Context, scope domain.ScopeType, scopeID string) ([]domain.FactConflict, error) {
	_ = ctx
	return s.repo.ListOpenFactConflicts(scope, scopeID, 100)
}

// --- Async / batch ----------------------------------------------------------

// BuildEmbedding asks the configured embedder for the fact's vector and
// stores it. Safe to call repeatedly; it overwrites the existing blob.
func (s *MemoryL3Service) BuildEmbedding(ctx context.Context, factID string) error {
	if s.embedder == nil {
		return errors.New("embedding source is not configured")
	}
	if factID == "" {
		return validationError("fact_id is required")
	}
	fact, err := s.repo.GetFact(factID)
	if err != nil {
		return err
	}
	text := strings.TrimSpace(fact.Statement)
	if fact.DetailsMarkdown != "" {
		text = text + "\n" + fact.DetailsMarkdown
	}
	vec, err := s.embedder.Embed(ctx, "", text)
	if err != nil {
		return err
	}
	blob := repository.EncodeFloat32Blob(vec)
	norm := vectorL2Norm(vec)
	return s.repo.UpsertFactEmbedding(factID, "", len(vec), blob, norm)
}

// RunDecayBatch scans for facts whose decay window expired and applies
// the §5.5 algorithm. Returns counts so the caller can emit metrics.
func (s *MemoryL3Service) RunDecayBatch(ctx context.Context) (DecayReport, error) {
	_ = ctx
	report := DecayReport{}
	now := s.now()
	facts, err := s.repo.ListFactsDueForDecay(now, 200)
	if err != nil {
		return report, err
	}
	threshold := 0.2
	intervalHours := 24
	report.Processed = len(facts)
	for _, f := range facts {
		factor := f.DecayFactor
		if factor <= 0 || factor >= 1 {
			factor = 0.98
		}
		newConf := clampUnit(f.Confidence * factor)
		report.ConfidenceDrop += f.Confidence - newConf
		nextAt := time.Now().UTC().Add(time.Duration(intervalHours) * time.Hour).Format(time.RFC3339)
		if newConf < threshold {
			_ = s.repo.UpdateFactStatus(f.ID, domain.FactStatusArchived, "", now)
			report.Archived++
			continue
		}
		_ = s.repo.ApplyFactDecay(f.ID, factor, nextAt)
	}
	if report.Archived == 0 && report.Processed > 0 {
		// also catch facts that fell below threshold via direct edit / feedback
		extra, _ := s.repo.ArchiveFactsBelowConfidence(threshold, 500)
		report.Archived += extra
	}
	return report, nil
}

// Stats returns the §6.6 admin/stats payload.
func (s *MemoryL3Service) Stats(ctx context.Context, scope domain.ScopeType, scopeID string) (L3StatsReport, error) {
	_ = ctx
	counts, err := s.repo.CountFactsByStatus(scope, scopeID)
	if err != nil {
		return L3StatsReport{}, err
	}
	return L3StatsReport{StatusCounts: counts}, nil
}

// --- L0 rendering -----------------------------------------------------------

// RenderForPrompt formats the recall hits as a system block ready to
// inject into the L0 assembly. The output content is bounded by maxChars
// so the L0 budget logic stays predictable.
func (s *MemoryL3Service) RenderForPrompt(ctx context.Context, hits []domain.FactRecallHit, maxChars int) (domain.FactPromptBlock, error) {
	_ = ctx
	if maxChars <= 0 {
		maxChars = 1500
	}
	if len(hits) == 0 {
		return domain.FactPromptBlock{Section: "memory.l3", Role: "system"}, nil
	}
	var b strings.Builder
	b.WriteString("Relevant long-term knowledge:\n")
	used := []domain.FactRecallHit{}
	for _, h := range hits {
		statement := h.Fact.Statement
		if h.Fact.PIIFlag && h.Fact.RedactedStatement != "" {
			statement = h.Fact.RedactedStatement
		}
		line := fmt.Sprintf("- [%s/%s · conf %.2f] %s\n", h.Fact.Kind, h.Fact.ScopeType, h.Fact.Confidence, statement)
		if b.Len()+len(line) > maxChars {
			break
		}
		b.WriteString(line)
		used = append(used, h)
	}
	content := strings.TrimRight(b.String(), "\n")
	return domain.FactPromptBlock{
		Section: "memory.l3",
		Role:    "system",
		Tokens:  estimateTokensApprox(content),
		Content: content,
		Items:   used,
	}, nil
}

// RecallSegmentForL0 is the seam consumed by MemoryL0Service. It honours
// the agent runtime settings (top_k / min_score / scopes / max_chars) so
// L0 doesn't need to know any L3 internals.
func (s *MemoryL3Service) RecallSegmentForL0(ctx context.Context, sessionID, agentID, query string) (domain.L0Segment, bool) {
	return s.RecallSegmentForL0WithContext(ctx, domain.L0MemoryScopeContext{
		SessionID: sessionID,
		AgentID:   agentID,
		Query:     query,
	})
}

// RecallSegmentForL0WithContext is the context-rich L0 seam. It includes
// user/team/workspace scope IDs so settings such as
// `l3_recall_scopes_json=["agent","team","workspace"]` work in normal chat.
func (s *MemoryL3Service) RecallSegmentForL0WithContext(ctx context.Context, scope domain.L0MemoryScopeContext) (domain.L0Segment, bool) {
	if strings.TrimSpace(scope.Query) == "" {
		return domain.L0Segment{}, false
	}
	settings, _ := s.repo.GetAgentRuntimeSettings(scope.AgentID)
	if !settings.L3Enabled {
		return domain.L0Segment{}, false
	}
	q := domain.FactRecallQuery{
		WorkspaceID:   scope.WorkspaceID,
		UserID:        scope.UserID,
		TeamID:        scope.TeamID,
		AgentID:       scope.AgentID,
		Query:         scope.Query,
		IncludeScopes: parseScopeList(settings.L3RecallScopesJSON),
		TopK:          firstPositive(settings.L3RecallTopK, 5),
		MinScore:      firstPositiveFloat(settings.L3RecallMinScore, 0.55),
		MaxChars:      firstPositive(settings.L3MaxPerRecallChars, 1500),
	}
	hits, err := s.Recall(ctx, q)
	if err != nil || len(hits) == 0 {
		return domain.L0Segment{}, false
	}
	block, err := s.RenderForPrompt(ctx, hits, q.MaxChars)
	if err != nil || strings.TrimSpace(block.Content) == "" {
		return domain.L0Segment{}, false
	}
	return domain.L0Segment{
		Section: block.Section,
		Role:    block.Role,
		Source:  fmt.Sprintf("memory.l3:%d", len(hits)),
		Tokens:  block.Tokens,
		Content: block.Content,
		Preview: previewText(block.Content, l0PreviewLimit),
	}, true
}

// --- internals --------------------------------------------------------------

func (s *MemoryL3Service) recordVersion(f domain.MemoryFact, reason, by string) error {
	if reason == "" {
		reason = "update"
	}
	return s.repo.InsertFactVersion(domain.FactVersion{
		ID:           newID(),
		FactID:       f.ID,
		Version:      f.Version,
		Statement:    f.Statement,
		Details:      f.DetailsMarkdown,
		Tags:         f.Tags,
		Confidence:   f.Confidence,
		Status:       f.Status,
		ChangedBy:    by,
		ChangeReason: reason,
		CreatedAt:    s.now(),
	})
}

func (s *MemoryL3Service) refreshFTS(f domain.MemoryFact) error {
	if f.Status != domain.FactStatusActive {
		return s.repo.UpsertFactsFTS(f.ID, f.ScopeType, f.ScopeID, string(f.Kind), "")
	}
	parts := []string{f.Statement}
	if f.DetailsMarkdown != "" {
		parts = append(parts, f.DetailsMarkdown)
	}
	if len(f.Tags) > 0 {
		parts = append(parts, strings.Join(f.Tags, " "))
	}
	return s.repo.UpsertFactsFTS(f.ID, f.ScopeType, f.ScopeID, string(f.Kind), strings.Join(parts, " "))
}

func (s *MemoryL3Service) extractFactToL4(ctx context.Context, factID string) {
	if s.memoryL4 == nil || factID == "" {
		return
	}
	report, err := s.memoryL4.ExtractFromFact(ctx, factID)
	if err != nil {
		_ = s.audit("memory.l3.l4_extract_failed", "memory_facts", factID, map[string]any{"error": err.Error()})
		return
	}
	_ = s.audit("memory.l3.l4_extract", "memory_facts", factID, map[string]any{
		"new_entities":     report.NewEntities,
		"updated_entities": report.UpdatedEntities,
		"errors":           report.Errors,
		"note":             report.Note,
	})
}

func (s *MemoryL3Service) audit(action, resource, resourceID string, detail map[string]any) error {
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
		CreatedAt:  s.now(),
	})
}

func (s *MemoryL3Service) markSelfConflict(f domain.MemoryFact) error {
	c := domain.FactConflict{
		FactAID:    f.ID,
		FactBID:    f.ID,
		ScopeType:  f.ScopeType,
		ScopeID:    f.ScopeID,
		Kind:       domain.FactConflictContradiction,
		Status:     domain.FactConflictStatusOpen,
		DetectedBy: "feedback_streak",
		Similarity: 1.0,
	}
	_, err := s.repo.UpsertFactConflict(c)
	return err
}

func (s *MemoryL3Service) expandScopes(includes []domain.ScopeType, q domain.FactRecallQuery) ([]domain.ScopeType, []string) {
	var scopes []domain.ScopeType
	var ids []string
	for _, sc := range includes {
		switch sc {
		case domain.ScopeAgent:
			if q.AgentID != "" {
				scopes = append(scopes, sc)
				ids = append(ids, q.AgentID)
			}
		case domain.ScopeUser:
			if q.UserID != "" {
				scopes = append(scopes, sc)
				ids = append(ids, q.UserID)
			}
		case domain.ScopeTeam:
			if q.TeamID != "" {
				scopes = append(scopes, sc)
				ids = append(ids, q.TeamID)
			}
		case domain.ScopeWorkspace:
			if q.WorkspaceID != "" {
				scopes = append(scopes, sc)
				ids = append(ids, q.WorkspaceID)
			}
		case domain.ScopeGlobal:
			scopes = append(scopes, sc)
			ids = append(ids, "")
		}
	}
	return scopes, ids
}

// mergeRecallHits unions two hit slices keyed on fact id, preserving the
// largest BM25 / vector score for each id.
func mergeRecallHits(bm, vec []domain.FactRecallHit) []domain.FactRecallHit {
	idx := map[string]*domain.FactRecallHit{}
	push := func(h domain.FactRecallHit) {
		cur, ok := idx[h.Fact.ID]
		if !ok {
			cp := h
			idx[h.Fact.ID] = &cp
			return
		}
		if h.BM25Score > cur.BM25Score {
			cur.BM25Score = h.BM25Score
		}
		if h.VectorScore > cur.VectorScore {
			cur.VectorScore = h.VectorScore
		}
		if h.Reason != "" && cur.Reason != "" && h.Reason != cur.Reason {
			cur.Reason = "hybrid"
		}
	}
	for _, h := range bm {
		push(h)
	}
	for _, h := range vec {
		push(h)
	}
	out := make([]domain.FactRecallHit, 0, len(idx))
	for _, p := range idx {
		out = append(out, *p)
	}
	return out
}

// applyRecallFilters drops hits whose fact doesn't match the requested
// tag / kind filters. The repository already filters scope + status.
func applyRecallFilters(hits []domain.FactRecallHit, q domain.FactRecallQuery) []domain.FactRecallHit {
	if len(q.Tags) == 0 && len(q.Kinds) == 0 {
		return hits
	}
	tagSet := map[string]bool{}
	for _, t := range q.Tags {
		tagSet[strings.ToLower(strings.TrimSpace(t))] = true
	}
	kindSet := map[domain.FactKind]bool{}
	for _, k := range q.Kinds {
		kindSet[k] = true
	}
	out := hits[:0]
	for _, h := range hits {
		if len(kindSet) > 0 && !kindSet[h.Fact.Kind] {
			continue
		}
		if len(tagSet) > 0 {
			match := false
			for _, t := range h.Fact.Tags {
				if tagSet[strings.ToLower(t)] {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		out = append(out, h)
	}
	return out
}

// scoreHits applies the §5.3 final-score formula in-place. BM25 scores
// from FTS5 are unbounded so we squash with a soft cap to 0..1; vector
// scores are already cosine-normalised.
func scoreHits(hits []domain.FactRecallHit, scopeWeights map[domain.ScopeType]float64, nowISOValue string) {
	now, _ := time.Parse(time.RFC3339, nowISOValue)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for i := range hits {
		h := &hits[i]
		bm := normaliseBM25(h.BM25Score)
		vec := h.VectorScore
		if vec < 0 {
			vec = 0
		}
		recency := recencyBoost(h.Fact.LastUsedAt, now)
		weight := scopeWeights[h.Fact.ScopeType]
		if weight == 0 {
			weight = 0.7
		}
		h.ScopeWeight = weight
		h.FinalScore = clampUnit(0.65*vec + 0.15*bm + 0.10*h.Fact.Confidence + 0.05*recency + 0.05*weight)
	}
}

func normaliseBM25(score float64) float64 {
	if score <= 0 {
		return 0
	}
	v := score / (score + 5.0)
	if v > 1 {
		v = 1
	}
	return v
}

func recencyBoost(lastUsed string, now time.Time) float64 {
	if lastUsed == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, lastUsed)
	if err != nil {
		return 0
	}
	delta := now.Sub(t).Hours() / 24
	if delta < 0 {
		delta = 0
	}
	// Half-life of ~30 days.
	boost := math.Exp(-delta / 30)
	if boost > 1 {
		boost = 1
	}
	return boost
}

// --- pure helpers -----------------------------------------------------------

func normalizeStatement(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 0x7F:
			b.WriteRune(r)
			prevSpace = false
		default:
			// drop punctuation
		}
	}
	return strings.TrimSpace(b.String())
}

func fingerprintForStatement(scope domain.ScopeType, scopeID, normalized string) string {
	h := sha256.Sum256([]byte(string(scope) + ":" + scopeID + ":" + normalized))
	return hex.EncodeToString(h[:])
}

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func dedupStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[strings.ToLower(v)] {
			continue
		}
		seen[strings.ToLower(v)] = true
		out = append(out, v)
	}
	return out
}

func mergeStringLists(a, b []string) []string {
	merged := append([]string{}, a...)
	merged = append(merged, b...)
	return dedupStrings(merged)
}

func chooseNonEmpty(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func encodeMetaJSON(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func inferScopeID(scope domain.ScopeType, in domain.FactUpsertInput) string {
	switch scope {
	case domain.ScopeAgent:
		return in.AgentID
	case domain.ScopeUser:
		return in.UserID
	case domain.ScopeTeam:
		return in.TeamID
	case domain.ScopeWorkspace:
		return in.WorkspaceID
	}
	return ""
}

func parseScopeList(raw string) []domain.ScopeType {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	out := make([]domain.ScopeType, 0, len(values))
	for _, v := range values {
		sc := domain.ScopeType(strings.ToLower(strings.TrimSpace(v)))
		if sc.IsValid() {
			out = append(out, sc)
		}
	}
	return out
}

func firstPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func firstPositiveFloat(value, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}

func vectorL2Norm(v []float32) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, f := range v {
		sum += float64(f) * float64(f)
	}
	return math.Sqrt(sum)
}
