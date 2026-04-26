package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"arenea/backend/internal/domain"
)

// memoryEntitySelectColumns is the canonical column list for SELECTs on
// memory_entities. Sharing it keeps Get / List / Neighborhood scans in
// lockstep with the migration schema.
const memoryEntitySelectColumns = `
	id, scope_type, scope_id, workspace_id, user_id,
	entity_type, name, name_normalized, aliases_json, description, attributes_json,
	importance, confidence, use_count, source_kind,
	embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
	status, merged_into,
	metadata_json, created_at, updated_at, archived_at, deleted_at
`

const memoryRelationSelectColumns = `
	id, scope_type, scope_id, workspace_id,
	source_id, target_id, relation_type, bidirectional,
	weight, confidence, importance, use_count,
	attributes_json, evidence_json, status, source_kind,
	metadata_json, created_at, updated_at, archived_at, deleted_at
`

func scanMemoryEntity(s scanner) (domain.MemoryEntity, error) {
	var (
		e             domain.MemoryEntity
		scopeType     string
		entityType    string
		aliasesJSON   string
		attributesJSON string
		metadataJSON  string
		embeddingBlob []byte
	)
	if err := s.Scan(
		&e.ID, &scopeType, &e.ScopeID, &e.WorkspaceID, &e.UserID,
		&entityType, &e.Name, &e.NameNormalized, &aliasesJSON, &e.Description, &attributesJSON,
		&e.Importance, &e.Confidence, &e.UseCount, &e.SourceKind,
		&e.EmbeddingStatus, &e.EmbeddingModel, &e.EmbeddingDim, &embeddingBlob, &e.EmbeddingNorm,
		&e.Status, &e.MergedInto,
		&metadataJSON, &e.CreatedAt, &e.UpdatedAt, &e.ArchivedAt, &e.DeletedAt,
	); err != nil {
		return domain.MemoryEntity{}, err
	}
	e.ScopeType = domain.ScopeType(scopeType)
	e.EntityType = domain.EntityType(entityType)
	e.EmbeddingBlob = embeddingBlob
	e.Aliases = decodeJSONStringSlice(aliasesJSON)
	e.Attributes = decodeJSONObject(attributesJSON)
	e.Metadata = decodeJSONObject(metadataJSON)
	return e, nil
}

func scanMemoryRelation(s scanner) (domain.MemoryRelation, error) {
	var (
		r              domain.MemoryRelation
		scopeType      string
		relationType   string
		bidirectional  int
		attributesJSON string
		evidenceJSON   string
		metadataJSON   string
	)
	if err := s.Scan(
		&r.ID, &scopeType, &r.ScopeID, &r.WorkspaceID,
		&r.SourceID, &r.TargetID, &relationType, &bidirectional,
		&r.Weight, &r.Confidence, &r.Importance, &r.UseCount,
		&attributesJSON, &evidenceJSON, &r.Status, &r.SourceKind,
		&metadataJSON, &r.CreatedAt, &r.UpdatedAt, &r.ArchivedAt, &r.DeletedAt,
	); err != nil {
		return domain.MemoryRelation{}, err
	}
	r.ScopeType = domain.ScopeType(scopeType)
	r.RelationType = domain.RelationType(relationType)
	r.Bidirectional = bidirectional != 0
	r.Attributes = decodeJSONObject(attributesJSON)
	r.Evidence = decodeEvidenceList(evidenceJSON)
	r.Metadata = decodeJSONObject(metadataJSON)
	return r, nil
}

func decodeJSONStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func decodeJSONObject(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func decodeEvidenceList(raw string) []domain.EvidenceRef {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []domain.EvidenceRef
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func encodeJSONStringSlice(in []string) string {
	if len(in) == 0 {
		return "[]"
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func encodeJSONObject(in map[string]any) string {
	if len(in) == 0 {
		return "{}"
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func encodeEvidenceList(in []domain.EvidenceRef) string {
	if len(in) == 0 {
		return "[]"
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// UpsertEntity persists or updates a memory_entities row keyed on
// (scope_type, scope_id, entity_type, name_normalized). When an entity
// with that natural key already exists the existing id is preserved and
// mutable fields are updated.
func (r *SQLiteRepository) UpsertEntity(e domain.MemoryEntity) (domain.MemoryEntity, error) {
	if e.ScopeType == "" {
		return domain.MemoryEntity{}, errors.New("entity scope_type is required")
	}
	if e.EntityType == "" {
		return domain.MemoryEntity{}, errors.New("entity_type is required")
	}
	if strings.TrimSpace(e.Name) == "" {
		return domain.MemoryEntity{}, errors.New("entity name is required")
	}
	if e.NameNormalized == "" {
		e.NameNormalized = strings.ToLower(strings.TrimSpace(e.Name))
	}
	if e.Status == "" {
		e.Status = domain.EntityStatusActive
	}
	if e.SourceKind == "" {
		e.SourceKind = domain.GraphSourceUser
	}
	if e.EmbeddingStatus == "" {
		e.EmbeddingStatus = "pending"
	}
	if e.Confidence == 0 {
		e.Confidence = 0.7
	}
	if e.Importance == 0 {
		e.Importance = 0.5
	}
	now := nowISO()
	if e.CreatedAt == "" {
		e.CreatedAt = now
	}
	e.UpdatedAt = now
	aliases := encodeJSONStringSlice(e.Aliases)
	attributes := encodeJSONObject(e.Attributes)
	metadata := encodeJSONObject(e.Metadata)

	if e.ID == "" {
		existing, err := r.GetEntityByName(e.ScopeType, e.ScopeID, e.EntityType, e.NameNormalized)
		if err == nil {
			e.ID = existing.ID
			e.CreatedAt = existing.CreatedAt
		} else if !errors.Is(err, sql.ErrNoRows) {
			return domain.MemoryEntity{}, err
		}
	}
	if e.ID == "" {
		return domain.MemoryEntity{}, errors.New("entity id is required (caller must populate)")
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_entities(
			id, scope_type, scope_id, workspace_id, user_id,
			entity_type, name, name_normalized, aliases_json, description, attributes_json,
			importance, confidence, use_count, source_kind,
			embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
			status, merged_into,
			metadata_json, created_at, updated_at, archived_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_id, entity_type, name_normalized) DO UPDATE SET
			workspace_id = excluded.workspace_id,
			user_id = excluded.user_id,
			name = excluded.name,
			aliases_json = excluded.aliases_json,
			description = excluded.description,
			attributes_json = excluded.attributes_json,
			importance = excluded.importance,
			confidence = excluded.confidence,
			source_kind = excluded.source_kind,
			embedding_status = excluded.embedding_status,
			embedding_model = excluded.embedding_model,
			embedding_dim = excluded.embedding_dim,
			embedding_blob = excluded.embedding_blob,
			embedding_norm = excluded.embedding_norm,
			status = excluded.status,
			merged_into = excluded.merged_into,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at,
			archived_at = excluded.archived_at,
			deleted_at = excluded.deleted_at`,
		e.ID, string(e.ScopeType), e.ScopeID, e.WorkspaceID, e.UserID,
		string(e.EntityType), e.Name, e.NameNormalized, aliases, e.Description, attributes,
		e.Importance, e.Confidence, e.UseCount, e.SourceKind,
		e.EmbeddingStatus, e.EmbeddingModel, e.EmbeddingDim, e.EmbeddingBlob, e.EmbeddingNorm,
		e.Status, e.MergedInto,
		metadata, e.CreatedAt, e.UpdatedAt, e.ArchivedAt, e.DeletedAt,
	)
	if err != nil {
		return domain.MemoryEntity{}, err
	}
	stored, err := r.GetEntity(e.ID)
	if err != nil {
		return domain.MemoryEntity{}, err
	}
	return stored, nil
}

func (r *SQLiteRepository) GetEntity(id string) (domain.MemoryEntity, error) {
	row := r.db.QueryRow(`SELECT `+memoryEntitySelectColumns+` FROM memory_entities WHERE id = ?`, id)
	return scanMemoryEntity(row)
}

func (r *SQLiteRepository) GetEntityByName(scope domain.ScopeType, scopeID string, t domain.EntityType, normalized string) (domain.MemoryEntity, error) {
	row := r.db.QueryRow(`SELECT `+memoryEntitySelectColumns+` FROM memory_entities WHERE scope_type = ? AND scope_id = ? AND entity_type = ? AND name_normalized = ?`, string(scope), scopeID, string(t), normalized)
	return scanMemoryEntity(row)
}

func (r *SQLiteRepository) ListEntities(q EntityListQuery) ([]domain.MemoryEntity, int, error) {
	conds := []string{}
	args := []any{}
	if q.ScopeType != "" {
		conds = append(conds, "scope_type = ?")
		args = append(args, string(q.ScopeType))
	}
	if q.ScopeID != "" {
		conds = append(conds, "scope_id = ?")
		args = append(args, q.ScopeID)
	}
	if q.WorkspaceID != "" {
		conds = append(conds, "workspace_id = ?")
		args = append(args, q.WorkspaceID)
	}
	if q.UserID != "" {
		conds = append(conds, "user_id = ?")
		args = append(args, q.UserID)
	}
	if q.EntityType != "" {
		conds = append(conds, "entity_type = ?")
		args = append(args, string(q.EntityType))
	}
	if q.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, q.Status)
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		conds = append(conds, "(LOWER(name) LIKE ? OR LOWER(description) LIKE ?)")
		like := "%" + strings.ToLower(kw) + "%"
		args = append(args, like, like)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM memory_entities`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, limit, offset)
	rows, err := r.db.Query(`SELECT `+memoryEntitySelectColumns+` FROM memory_entities`+where+` ORDER BY importance DESC, updated_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.MemoryEntity{}
	for rows.Next() {
		v, err := scanMemoryEntity(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

func (r *SQLiteRepository) UpdateEntityStatus(id, status, mergedInto, archivedAt, deletedAt string) error {
	if id == "" {
		return errors.New("entity id is required")
	}
	_, err := r.db.Exec(
		`UPDATE memory_entities SET status = ?, merged_into = ?, archived_at = ?, deleted_at = ?, updated_at = ? WHERE id = ?`,
		status, mergedInto, archivedAt, deletedAt, nowISO(), id,
	)
	return err
}

func (r *SQLiteRepository) UpdateEntityName(id, name, normalized string) error {
	if id == "" {
		return errors.New("entity id is required")
	}
	if name == "" {
		return errors.New("entity name is required")
	}
	if normalized == "" {
		normalized = strings.ToLower(strings.TrimSpace(name))
	}
	_, err := r.db.Exec(
		`UPDATE memory_entities SET name = ?, name_normalized = ?, updated_at = ? WHERE id = ?`,
		name, normalized, nowISO(), id,
	)
	return err
}

func (r *SQLiteRepository) UpsertEntityFact(entityID, factID string, weight float64) error {
	if entityID == "" || factID == "" {
		return errors.New("entity_id and fact_id are required")
	}
	if weight <= 0 {
		weight = 1.0
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_entity_facts(entity_id, fact_id, weight, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(entity_id, fact_id) DO UPDATE SET weight = excluded.weight`,
		entityID, factID, weight, nowISO(),
	)
	return err
}

func (r *SQLiteRepository) ListFactsForEntity(entityID string, limit int) ([]domain.MemoryEntityFactLink, error) {
	if entityID == "" {
		return nil, errors.New("entity id is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := r.db.Query(
		`SELECT entity_id, fact_id, weight, created_at FROM memory_entity_facts WHERE entity_id = ? ORDER BY weight DESC, created_at DESC LIMIT ?`,
		entityID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.MemoryEntityFactLink{}
	for rows.Next() {
		var v domain.MemoryEntityFactLink
		if err := rows.Scan(&v.EntityID, &v.FactID, &v.Weight, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) InsertEntityVersion(v domain.MemoryEntityVersion) error {
	if v.ID == "" {
		return errors.New("version id is required")
	}
	if v.EntityID == "" {
		return errors.New("entity id is required")
	}
	if v.SnapshotJSON == "" {
		return errors.New("snapshot is required")
	}
	if v.CreatedAt == "" {
		v.CreatedAt = nowISO()
	}
	if v.DiffJSON == "" {
		v.DiffJSON = "{}"
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_entity_versions(id, entity_id, version, snapshot_json, changed_by, change_reason, diff_json, metadata_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.EntityID, v.Version, v.SnapshotJSON, v.ChangedBy, v.ChangeReason, v.DiffJSON, encodeJSONObject(v.Metadata), v.CreatedAt,
	)
	return err
}

func (r *SQLiteRepository) ListEntityVersions(entityID string, limit int) ([]domain.MemoryEntityVersion, error) {
	if entityID == "" {
		return nil, errors.New("entity id is required")
	}
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	rows, err := r.db.Query(
		`SELECT id, entity_id, version, snapshot_json, changed_by, change_reason, diff_json, metadata_json, created_at
		 FROM memory_entity_versions WHERE entity_id = ? ORDER BY version DESC LIMIT ?`,
		entityID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.MemoryEntityVersion{}
	for rows.Next() {
		var v domain.MemoryEntityVersion
		var metadata string
		if err := rows.Scan(&v.ID, &v.EntityID, &v.Version, &v.SnapshotJSON, &v.ChangedBy, &v.ChangeReason, &v.DiffJSON, &metadata, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.Metadata = decodeJSONObject(metadata)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) BumpEntityUseCount(id string, atISO string) error {
	if id == "" {
		return errors.New("entity id is required")
	}
	if atISO == "" {
		atISO = nowISO()
	}
	_, err := r.db.Exec(`UPDATE memory_entities SET use_count = use_count + 1, updated_at = ? WHERE id = ?`, atISO, id)
	return err
}

// UpsertRelation persists or updates a memory_relations row keyed on
// (scope_type, scope_id, source_id, target_id, relation_type).
func (r *SQLiteRepository) UpsertRelation(rel domain.MemoryRelation) (domain.MemoryRelation, error) {
	if rel.ScopeType == "" {
		return domain.MemoryRelation{}, errors.New("relation scope_type is required")
	}
	if rel.SourceID == "" || rel.TargetID == "" {
		return domain.MemoryRelation{}, errors.New("relation source_id and target_id are required")
	}
	if rel.RelationType == "" {
		return domain.MemoryRelation{}, errors.New("relation_type is required")
	}
	if rel.Status == "" {
		rel.Status = domain.RelationStatusActive
	}
	if rel.SourceKind == "" {
		rel.SourceKind = domain.GraphSourceUser
	}
	if rel.Weight == 0 {
		rel.Weight = 1.0
	}
	if rel.Confidence == 0 {
		rel.Confidence = 0.7
	}
	if rel.Importance == 0 {
		rel.Importance = 0.5
	}
	now := nowISO()
	if rel.CreatedAt == "" {
		rel.CreatedAt = now
	}
	rel.UpdatedAt = now
	if rel.ID == "" {
		return domain.MemoryRelation{}, errors.New("relation id is required (caller must populate)")
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_relations(
			id, scope_type, scope_id, workspace_id,
			source_id, target_id, relation_type, bidirectional,
			weight, confidence, importance, use_count,
			attributes_json, evidence_json, status, source_kind,
			metadata_json, created_at, updated_at, archived_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_id, source_id, target_id, relation_type) DO UPDATE SET
			workspace_id = excluded.workspace_id,
			bidirectional = excluded.bidirectional,
			weight = excluded.weight,
			confidence = excluded.confidence,
			importance = excluded.importance,
			attributes_json = excluded.attributes_json,
			evidence_json = excluded.evidence_json,
			status = excluded.status,
			source_kind = excluded.source_kind,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at,
			archived_at = excluded.archived_at,
			deleted_at = excluded.deleted_at`,
		rel.ID, string(rel.ScopeType), rel.ScopeID, rel.WorkspaceID,
		rel.SourceID, rel.TargetID, string(rel.RelationType), boolToInt(rel.Bidirectional),
		rel.Weight, rel.Confidence, rel.Importance, rel.UseCount,
		encodeJSONObject(rel.Attributes), encodeEvidenceList(rel.Evidence), rel.Status, rel.SourceKind,
		encodeJSONObject(rel.Metadata), rel.CreatedAt, rel.UpdatedAt, rel.ArchivedAt, rel.DeletedAt,
	)
	if err != nil {
		return domain.MemoryRelation{}, err
	}
	return r.GetRelation(rel.ID)
}

func (r *SQLiteRepository) GetRelation(id string) (domain.MemoryRelation, error) {
	row := r.db.QueryRow(`SELECT `+memoryRelationSelectColumns+` FROM memory_relations WHERE id = ?`, id)
	return scanMemoryRelation(row)
}

func (r *SQLiteRepository) ListRelationsForNode(nodeID string, limit int) ([]domain.MemoryRelation, error) {
	if nodeID == "" {
		return nil, errors.New("node id is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(
		`SELECT `+memoryRelationSelectColumns+` FROM memory_relations
		 WHERE (source_id = ? OR target_id = ?) AND status = 'active'
		 ORDER BY weight DESC, updated_at DESC LIMIT ?`,
		nodeID, nodeID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.MemoryRelation{}
	for rows.Next() {
		v, err := scanMemoryRelation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) UpdateRelationStatus(id, status, archivedAt, deletedAt string) error {
	if id == "" {
		return errors.New("relation id is required")
	}
	_, err := r.db.Exec(
		`UPDATE memory_relations SET status = ?, archived_at = ?, deleted_at = ?, updated_at = ? WHERE id = ?`,
		status, archivedAt, deletedAt, nowISO(), id,
	)
	return err
}

func (r *SQLiteRepository) BumpRelationUseCount(id string, atISO string) error {
	if id == "" {
		return errors.New("relation id is required")
	}
	if atISO == "" {
		atISO = nowISO()
	}
	_, err := r.db.Exec(`UPDATE memory_relations SET use_count = use_count + 1, updated_at = ? WHERE id = ?`, atISO, id)
	return err
}

// GetNeighborhood traverses up to `hops` hops outward from `centerID`,
// returning at most `maxNodes` distinct entities (excluding the center)
// plus the relations that connect them. Performed in pure Go using
// repeated SELECTs to keep portability across SQLite installations that
// lack `WITH RECURSIVE` quirks.
func (r *SQLiteRepository) GetNeighborhood(centerID string, hops, maxNodes int) (domain.GraphNeighborhood, error) {
	center, err := r.GetEntity(centerID)
	if err != nil {
		return domain.GraphNeighborhood{}, err
	}
	if hops <= 0 {
		hops = 1
	}
	if hops > 3 {
		hops = 3
	}
	if maxNodes <= 0 || maxNodes > 200 {
		maxNodes = 25
	}

	visited := map[string]bool{centerID: true}
	frontier := []string{centerID}
	relSeen := map[string]bool{}
	entities := []domain.MemoryEntity{}
	relations := []domain.MemoryRelation{}

	for h := 0; h < hops && len(visited) < maxNodes+1 && len(frontier) > 0; h++ {
		next := []string{}
		for _, node := range frontier {
			rels, err := r.ListRelationsForNode(node, 100)
			if err != nil {
				return domain.GraphNeighborhood{}, err
			}
			for _, rel := range rels {
				if relSeen[rel.ID] {
					continue
				}
				relSeen[rel.ID] = true
				relations = append(relations, rel)
				other := rel.TargetID
				if other == node {
					other = rel.SourceID
				}
				if other == "" || visited[other] {
					continue
				}
				if len(visited) >= maxNodes+1 {
					continue
				}
				visited[other] = true
				ent, err := r.GetEntity(other)
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						continue
					}
					return domain.GraphNeighborhood{}, err
				}
				entities = append(entities, ent)
				next = append(next, other)
			}
		}
		frontier = next
	}

	return domain.GraphNeighborhood{
		Center:    center,
		Hops:      hops,
		Entities:  entities,
		Relations: relations,
	}, nil
}

