// Package domain – memory L4 persistent / evolutionary memory types
// described in `aranea/docs/16 memory-L4-persistent.md`. L4 stores a long-
// lived knowledge graph (entities + relations) plus the agent's evolving
// identity / strategy profile and is the topmost layer of the 5-layer
// memory architecture.
package domain

// EntityType classifies a knowledge-graph node. Strings persist in
// `memory_entities.entity_type` so changing values requires a migration.
type EntityType string

const (
	EntityPerson     EntityType = "person"
	EntityProject    EntityType = "project"
	EntityRepository EntityType = "repository"
	EntityTech       EntityType = "tech"
	EntityFramework  EntityType = "framework"
	EntityCompany    EntityType = "company"
	EntityTopic      EntityType = "topic"
	EntityFile       EntityType = "file"
	EntityEndpoint   EntityType = "endpoint"
	EntityCustom     EntityType = "custom"
)

// IsValid reports whether the entity type is one of the known enum values.
// Unknown types are still allowed via EntityCustom; callers may relax this
// check when accepting plugin-supplied taxonomies.
func (t EntityType) IsValid() bool {
	switch t {
	case EntityPerson, EntityProject, EntityRepository, EntityTech, EntityFramework,
		EntityCompany, EntityTopic, EntityFile, EntityEndpoint, EntityCustom:
		return true
	}
	return false
}

// RelationType classifies a knowledge-graph edge. Strings persist in
// `memory_relations.relation_type`.
type RelationType string

const (
	RelWorksOn    RelationType = "works_on"
	RelUses       RelationType = "uses"
	RelDependsOn  RelationType = "depends_on"
	RelAuthoredBy RelationType = "authored_by"
	RelPartOf     RelationType = "part_of"
	RelSimilarTo  RelationType = "similar_to"
	RelReplaces   RelationType = "replaces"
	RelMemberOf   RelationType = "member_of"
	RelSupports   RelationType = "supports"
	RelBlocks     RelationType = "blocks"
)

// IsValid reports whether the relation type is one of the known enum
// values. Custom types are allowed but should be vetted upstream.
func (r RelationType) IsValid() bool {
	switch r {
	case RelWorksOn, RelUses, RelDependsOn, RelAuthoredBy, RelPartOf,
		RelSimilarTo, RelReplaces, RelMemberOf, RelSupports, RelBlocks:
		return true
	}
	return false
}

// Entity / Relation status values persisted in `status` columns.
const (
	EntityStatusActive   = "active"
	EntityStatusMerged   = "merged"
	EntityStatusArchived = "archived"
	EntityStatusDeleted  = "deleted"

	RelationStatusActive   = "active"
	RelationStatusArchived = "archived"
	RelationStatusDeleted  = "deleted"
)

// Source kinds describe how an entity / relation was created.
const (
	GraphSourceExtracted   = "extracted"
	GraphSourceUser        = "user"
	GraphSourcePlugin      = "plugin"
	GraphSourceAgent       = "agent"
	GraphSourceConsolidate = "consolidator"
)

// EvidenceRef is a structured reference to a fact / episode / message that
// supports an entity, relation, or evolution change. Persisted as a JSON
// element inside the various `evidence_json` columns.
type EvidenceRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// MemoryEntity is the persisted row in `memory_entities`. JSON tags match
// the wire format used by the §6.2 HTTP API.
type MemoryEntity struct {
	ID             string         `json:"id"`
	ScopeType      ScopeType      `json:"scope_type"`
	ScopeID        string         `json:"scope_id"`
	WorkspaceID    string         `json:"workspace_id,omitempty"`
	UserID         string         `json:"user_id,omitempty"`
	EntityType     EntityType     `json:"entity_type"`
	Name           string         `json:"name"`
	NameNormalized string         `json:"name_normalized,omitempty"`
	Aliases        []string       `json:"aliases"`
	Description    string         `json:"description,omitempty"`
	Attributes     map[string]any `json:"attributes,omitempty"`

	Importance float64 `json:"importance"`
	Confidence float64 `json:"confidence"`
	UseCount   int     `json:"use_count"`
	SourceKind string  `json:"source_kind,omitempty"`

	EmbeddingStatus string  `json:"embedding_status,omitempty"`
	EmbeddingModel  string  `json:"embedding_model,omitempty"`
	EmbeddingDim    int     `json:"embedding_dim,omitempty"`
	EmbeddingBlob   []byte  `json:"-"`
	EmbeddingNorm   float64 `json:"embedding_norm,omitempty"`

	Status     string `json:"status"`
	MergedInto string `json:"merged_into,omitempty"`

	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  string         `json:"created_at,omitempty"`
	UpdatedAt  string         `json:"updated_at,omitempty"`
	ArchivedAt string         `json:"archived_at,omitempty"`
	DeletedAt  string         `json:"deleted_at,omitempty"`
}

// MemoryRelation is the persisted row in `memory_relations`.
type MemoryRelation struct {
	ID          string    `json:"id"`
	ScopeType   ScopeType `json:"scope_type"`
	ScopeID     string    `json:"scope_id"`
	WorkspaceID string    `json:"workspace_id,omitempty"`

	SourceID      string       `json:"source_id"`
	TargetID      string       `json:"target_id"`
	RelationType  RelationType `json:"relation_type"`
	Bidirectional bool         `json:"bidirectional,omitempty"`

	Weight     float64 `json:"weight"`
	Confidence float64 `json:"confidence"`
	Importance float64 `json:"importance"`
	UseCount   int     `json:"use_count"`

	Attributes map[string]any `json:"attributes,omitempty"`
	Evidence   []EvidenceRef  `json:"evidence,omitempty"`
	Status     string         `json:"status"`
	SourceKind string         `json:"source_kind,omitempty"`

	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  string         `json:"created_at,omitempty"`
	UpdatedAt  string         `json:"updated_at,omitempty"`
	ArchivedAt string         `json:"archived_at,omitempty"`
	DeletedAt  string         `json:"deleted_at,omitempty"`
}

// MemoryEntityVersion captures one historical snapshot of a
// `memory_entities` row. `change_reason` is one of create / update /
// merge / split / rename / restore.
type MemoryEntityVersion struct {
	ID           string         `json:"id"`
	EntityID     string         `json:"entity_id"`
	Version      int            `json:"version"`
	SnapshotJSON string         `json:"snapshot_json"`
	ChangedBy    string         `json:"changed_by,omitempty"`
	ChangeReason string         `json:"change_reason,omitempty"`
	DiffJSON     string         `json:"diff_json,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    string         `json:"created_at,omitempty"`
}

// MemoryEntityFactLink is the entity ↔ L3 fact reverse index row.
type MemoryEntityFactLink struct {
	EntityID  string  `json:"entity_id"`
	FactID    string  `json:"fact_id"`
	Weight    float64 `json:"weight"`
	CreatedAt string  `json:"created_at,omitempty"`
}

// GraphNeighborhood is the result of `MemoryL4GraphService.Neighborhood`.
// `Hops` is the actual hop count returned (capped by the request).
type GraphNeighborhood struct {
	Center    MemoryEntity     `json:"center"`
	Hops      int              `json:"hops"`
	Entities  []MemoryEntity   `json:"entities"`
	Relations []MemoryRelation `json:"relations"`
}
