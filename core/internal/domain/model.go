package domain

import "time"

// ID is the stable identifier used throughout the domain.
type ID string

type Project struct {
	ID        ID        `json:"id"`
	Name      string    `json:"name"`
	Root      string    `json:"root"`
	Includes  []string  `json:"includes,omitempty"`
	Excludes  []string  `json:"excludes,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Revision struct {
	ID        ID        `json:"id"`
	ProjectID ID        `json:"projectId"`
	Digest    string    `json:"digest"`
	CreatedAt time.Time `json:"createdAt"`
}

type Artifact struct {
	ID          ID     `json:"id"`
	ProjectID   ID     `json:"projectId"`
	RevisionID  ID     `json:"revisionId"`
	Path        string `json:"path"`
	Language    string `json:"language"`
	Origin      string `json:"origin"`
	ContentHash string `json:"contentHash"`
}

type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type Symbol struct {
	ID            ID        `json:"id"`
	ProjectID     ID        `json:"projectId"`
	RevisionID    ID        `json:"revisionId"`
	ArtifactID    ID        `json:"artifactId"`
	Path          string    `json:"path"`
	QualifiedName string    `json:"qualifiedName"`
	Descriptor    string    `json:"descriptor"`
	Kind          string    `json:"kind"`
	Location      *Location `json:"location,omitempty"`
}

type Resolution string

const (
	ResolutionResolved   Resolution = "resolved"
	ResolutionDynamic    Resolution = "dynamic"
	ResolutionUnresolved Resolution = "unresolved"
)

type Layer string

const (
	LayerStatic   Layer = "static"
	LayerObserved Layer = "observed"
)

type Relation struct {
	ID          ID         `json:"id"`
	FromID      ID         `json:"fromId"`
	ToID        *ID        `json:"toId,omitempty"`
	Kind        string     `json:"kind"`
	EvidenceIDs []ID       `json:"evidenceIds"`
	Resolution  Resolution `json:"resolution"`
	Layer       Layer      `json:"layer"`
	Location    *Location  `json:"location,omitempty"`
}

type Evidence struct {
	ID         ID         `json:"id"`
	Kind       string     `json:"kind"`
	Source     string     `json:"source"`
	Location   *Location  `json:"location,omitempty"`
	ObservedAt *time.Time `json:"observedAt,omitempty"`
}

type Interaction struct {
	ID         ID        `json:"id"`
	ProjectID  ID        `json:"projectId"`
	TraceID    ID        `json:"traceId"`
	OccurredAt time.Time `json:"occurredAt"`
	EventIDs   []ID      `json:"eventIds"`
}

type Trace struct {
	ID         ID         `json:"id"`
	ProjectID  ID         `json:"projectId"`
	StartedAt  time.Time  `json:"startedAt"`
	EndedAt    *time.Time `json:"endedAt,omitempty"`
	Incomplete bool       `json:"incomplete,omitempty"`
}

type Event struct {
	ProjectID           ID                `json:"projectId"`
	TraceID             ID                `json:"traceId"`
	ProducerID          ID                `json:"producerId"`
	Sequence            uint64            `json:"sequence"`
	EventID             ID                `json:"eventId"`
	ParentEventID       *ID               `json:"parentEventId,omitempty"`
	Kind                string            `json:"kind"`
	OccurredAt          time.Time         `json:"occurredAt"`
	ApplicationRevision *ID               `json:"applicationRevision,omitempty"`
	JavaDestination     *ID               `json:"javaDestination,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
}

type Diagnostic struct {
	ID        ID        `json:"id"`
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Severity  string    `json:"severity"`
	Location  *Location `json:"location,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
