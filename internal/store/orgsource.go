package store

import "context"

// OrgDocument is one organizational ground-truth record from an external source.
// Future wiki/plugin adapters implement OrgSource and upsert by ExternalRef.
type OrgDocument struct {
	ExternalRef string
	SourceURI   string
	Content     string
	Category    string
	Importance  int
	Tags        []string
	Entities    []string
}

// OrgSource is a future plugin hook for organizational memories stored outside
// Postgres (for example a company wiki). Mnemon would list documents, upsert
// them as layer=org insights keyed by ExternalRef, then recall and adjust them
// like any other memory. Not implemented in this cut.
type OrgSource interface {
	List(ctx context.Context) ([]OrgDocument, error)
}
