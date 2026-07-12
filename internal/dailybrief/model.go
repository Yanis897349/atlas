package dailybrief

import (
	"context"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

// MaxStoredBriefsLimit bounds one stored-daily-brief retrieval.
const MaxStoredBriefsLimit = 100

// CitationKind identifies the source entity cited by a section.
type CitationKind string

const (
	CitationSourceRecord  CitationKind = "source_record"
	CitationUpcomingEvent CitationKind = "upcoming_event"
)

// CitationReference identifies one input item selected by a generator.
type CitationReference struct {
	Kind CitationKind
	ID   string
}

// SectionDraft is provider output before citation metadata is resolved.
type SectionDraft struct {
	Heading   string
	Content   string
	Citations []CitationReference
}

// Draft is provider output before citation metadata is resolved.
type Draft struct {
	Sections []SectionDraft
}

// Generation contains provider output and its provenance.
type Generation struct {
	Provider string
	Model    string
	Draft    Draft
}

// Citation contains canonical metadata resolved from the generation input.
type Citation struct {
	Kind   CitationKind
	ID     string
	Source string
	URL    string
}

// Section is a generated section with canonical citations.
type Section struct {
	Heading   string
	Content   string
	Citations []Citation
}

// Brief is a validated daily brief with generation provenance.
type Brief struct {
	Region                 calendar.Region
	PublicationWindowStart time.Time
	PublicationWindowEnd   time.Time
	EventWindowStart       time.Time
	EventWindowEnd         time.Time
	Provider               string
	Model                  string
	Sections               []Section
}

// StoredBrief is a daily brief with persistence identity and audit metadata.
type StoredBrief struct {
	ID string
	Brief
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
}

// Generator produces a provider draft from deterministic input.
type Generator interface {
	Generate(context.Context, Input) (Generation, error)
}

// Persistence atomically stores a validated daily brief.
type Persistence interface {
	PersistDailyBrief(context.Context, Brief, string) (StoredBrief, error)
}

// Reader retrieves stored daily briefs.
type Reader interface {
	StoredDailyBriefs(context.Context, calendar.Region, time.Time, time.Time, int) ([]StoredBrief, error)
}
